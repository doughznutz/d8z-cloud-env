package jobs

// Native GCP: Compute Engine REST calls straight from gw-server, authorized
// by a service-account key (deployment.yaml gcp.credentials, a file in the
// config dir). Replaces the old dev/cloud gcloud container - same idea as
// GitHub and Docker Hub: no SDK, just HTTP.
//
// A cloud target is one Container-Optimized OS VM (d8z-<name>) whose
// startup script (cloud-startup.sh, run on every boot) pulls our gateway
// image from Docker Hub and brings up the target's compose profile from the
// docker-compose.yaml baked into it. Everything the VM needs (the .env, the
// image, the profile) rides in instance metadata.

import (
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	_ "embed"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"d8z/gw/deployment"
	"d8z/gw/internal/apiclient"
)

// cloudStartupScript is the VM's startup-script metadata (cloud-startup.sh).
//
//go:embed cloud-startup.sh
var cloudStartupScript string

// computeAPI is the Compute Engine v1 REST base URL.
const computeAPI = "https://compute.googleapis.com/compute/v1"

// gcpClient is an authorized Compute Engine client for one project and zone.
type gcpClient struct {
	project, zone string
	email         string
	hdr           http.Header
}

// newGCP signs a JWT with the service account's key and exchanges it for
// an access token (OAuth 2.0 JWT bearer grant).
func (c *Catalog) newGCP(ctx context.Context, d *deployment.Deployment) (*gcpClient, error) {
	g := d.GCP
	if g.Project == "" || g.Zone == "" || g.Credentials == "" {
		return nil, fmt.Errorf("deployment.yaml needs gcp.project, gcp.zone and gcp.credentials (a service-account key file in %s)", c.deploy.Dir())
	}
	keyFile := filepath.Join(c.deploy.Dir(), g.Credentials)
	b, err := os.ReadFile(keyFile)
	if err != nil {
		return nil, fmt.Errorf("GCP credentials: %v", err)
	}
	var key struct {
		Type        string `json:"type"`
		ClientEmail string `json:"client_email"`
		PrivateKey  string `json:"private_key"`
		TokenURI    string `json:"token_uri"`
	}
	if err := json.Unmarshal(b, &key); err != nil || key.Type != "service_account" {
		return nil, fmt.Errorf("%s is not a service-account key JSON", keyFile)
	}
	blk, _ := pem.Decode([]byte(key.PrivateKey))
	if blk == nil {
		return nil, fmt.Errorf("%s: no PEM private key", keyFile)
	}
	pk, err := x509.ParsePKCS8PrivateKey(blk.Bytes)
	if err != nil {
		return nil, fmt.Errorf("%s: private key: %v", keyFile, err)
	}
	rsaKey, ok := pk.(*rsa.PrivateKey)
	if !ok {
		return nil, fmt.Errorf("%s: not an RSA key", keyFile)
	}
	tokenURI := firstNonEmpty(key.TokenURI, "https://oauth2.googleapis.com/token")
	now := time.Now()
	enc := func(v any) string {
		j, _ := json.Marshal(v)
		return base64.RawURLEncoding.EncodeToString(j)
	}
	unsigned := enc(map[string]string{"alg": "RS256", "typ": "JWT"}) + "." + enc(map[string]any{
		"iss": key.ClientEmail, "scope": "https://www.googleapis.com/auth/compute",
		"aud": tokenURI, "iat": now.Unix(), "exp": now.Add(time.Hour).Unix(),
	})
	sum := sha256.Sum256([]byte(unsigned))
	sig, err := rsa.SignPKCS1v15(rand.Reader, rsaKey, crypto.SHA256, sum[:])
	if err != nil {
		return nil, err
	}
	form := url.Values{
		"grant_type": {"urn:ietf:params:oauth:grant-type:jwt-bearer"},
		"assertion":  {unsigned + "." + base64.RawURLEncoding.EncodeToString(sig)},
	}
	req, _ := http.NewRequestWithContext(ctx, "POST", tokenURI, strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := apiclient.HTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("GCP token: %v", err)
	}
	defer resp.Body.Close()
	var tok struct {
		AccessToken string `json:"access_token"`
		Error       string `json:"error"`
		Desc        string `json:"error_description"`
	}
	json.NewDecoder(resp.Body).Decode(&tok)
	if tok.AccessToken == "" {
		return nil, fmt.Errorf("GCP token for %s: %s %s (HTTP %d)", key.ClientEmail, tok.Error, tok.Desc, resp.StatusCode)
	}
	hdr := http.Header{}
	hdr.Set("Authorization", "Bearer "+tok.AccessToken)
	return &gcpClient{project: g.Project, zone: g.Zone, email: key.ClientEmail, hdr: hdr}, nil
}

// do makes an authorized JSON call. path is relative to the project
// ("/zones/..."), or a full https:// URL such as an operation's selfLink.
func (c *gcpClient) do(ctx context.Context, method, path string, body, out any) error {
	u := path
	if !strings.HasPrefix(u, "https://") {
		u = computeAPI + "/projects/" + c.project + path
	}
	return apiclient.DoJSON(ctx, method, u, c.hdr, body, out)
}

// gceOperation is a Compute Engine long-running operation.
type gceOperation struct {
	Name     string `json:"name"`
	SelfLink string `json:"selfLink"`
	Status   string `json:"status"`
	Error    *struct {
		Errors []struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"errors"`
	} `json:"error"`
}

// wait blocks until a zone/global operation is DONE, returning its error.
func (c *gcpClient) wait(ctx context.Context, op gceOperation) error {
	for op.Status != "DONE" {
		if op.SelfLink == "" {
			return fmt.Errorf("GCP operation without a selfLink")
		}
		var next gceOperation
		if err := c.do(ctx, "POST", op.SelfLink+"/wait", nil, &next); err != nil {
			return err
		}
		op = next
	}
	if op.Error != nil && len(op.Error.Errors) > 0 {
		var msgs []string
		for _, e := range op.Error.Errors {
			msgs = append(msgs, e.Code+": "+e.Message)
		}
		return fmt.Errorf("GCP: %s", strings.Join(msgs, "; "))
	}
	return nil
}

// call runs one mutating request and waits for its operation.
func (c *gcpClient) call(ctx context.Context, method, path string, body any) error {
	var op gceOperation
	if err := c.do(ctx, method, path, body, &op); err != nil {
		return err
	}
	return c.wait(ctx, op)
}

// gceInstance is the subset of a Compute Engine instance we read.
type gceInstance struct {
	Name              string `json:"name"`
	Status            string `json:"status"`
	NetworkInterfaces []struct {
		AccessConfigs []struct {
			NatIP string `json:"natIP"`
		} `json:"accessConfigs"`
	} `json:"networkInterfaces"`
	Metadata struct {
		Fingerprint string `json:"fingerprint"`
	} `json:"metadata"`
	Labels map[string]string `json:"labels"`
}

// address is the VM's external (NAT) IP, or "" while it has none.
func (i *gceInstance) address() string {
	for _, n := range i.NetworkInterfaces {
		for _, a := range n.AccessConfigs {
			if a.NatIP != "" {
				return a.NatIP
			}
		}
	}
	return ""
}

// instancePath is instance name's API path in our zone.
func (c *gcpClient) instancePath(name string) string {
	return "/zones/" + c.zone + "/instances/" + name
}

// instance returns the VM, or nil if it doesn't exist.
func (c *gcpClient) instance(ctx context.Context, name string) (*gceInstance, error) {
	var in gceInstance
	err := c.do(ctx, "GET", c.instancePath(name), nil, &in)
	if apiclient.IsStatus(err, 404) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &in, nil
}

// vmName is the GCE instance name of a cloud target.
func vmName(target string) string { return "d8z-" + target }

// firewallName is the firewall rule (and network tag) of a cloud target.
func firewallName(target string) string { return "d8z-gw-" + target }

// ensureFirewall opens :80 and the NATS leaf port to the target's
// allow_from ranges - and only on its own VM (by network tag).
func (c *gcpClient) ensureFirewall(ctx context.Context, j *jobRun, t deployment.CloudTarget) error {
	if len(t.AllowFrom) == 0 {
		return fmt.Errorf("targets[%s].allow_from is empty: list the CIDRs that may reach the gateway (e.g. your public IP/32)", t.Name)
	}
	fw := map[string]any{
		"name": firewallName(t.Name), "network": "global/networks/default", "direction": "INGRESS",
		"description":  "d8z gateway " + t.Name + ": console (80) and NATS leaf (7422)",
		"sourceRanges": t.AllowFrom, "targetTags": []string{firewallName(t.Name)},
		"allowed": []map[string]any{{"IPProtocol": "tcp", "ports": []string{"80", fmt.Sprint(deployment.CloudLeafPort)}}},
	}
	path := "/global/firewalls/" + firewallName(t.Name)
	err := c.do(ctx, "GET", path, nil, nil)
	if apiclient.IsStatus(err, 404) {
		j.logf("creating firewall rule %s (tcp 80, %d from %s)", firewallName(t.Name), deployment.CloudLeafPort, strings.Join(t.AllowFrom, ", "))
		return c.call(ctx, "POST", "/global/firewalls", fw)
	}
	if err != nil {
		return err
	}
	j.logf("updating firewall rule %s (from %s)", firewallName(t.Name), strings.Join(t.AllowFrom, ", "))
	return c.call(ctx, "PATCH", path, fw)
}

// cloudEnv is the .env the VM's compose project runs with: who it is, how
// it's reached, and what it pulls - plus any forward_env keys. Values come
// from our own environment (this gateway's .env).
func (c *Catalog) cloudEnv(d *deployment.Deployment, t deployment.CloudTarget) (string, error) {
	pw := os.Getenv("DOCKER_PASSWORD")
	if pw == "" {
		return "", fmt.Errorf("DOCKER_PASSWORD (the console's basic-auth password) is unset in .env: the cloud gateway would refuse to start")
	}
	kv := [][2]string{
		{"USER", firstNonEmpty(os.Getenv("USER"), "d8z")},
		{"PROJECT", firstNonEmpty(defaultProject(), "workspace")},
		{"DOCKER_PASSWORD", pw},
		{"CODE_SERVER_PASSWORD", os.Getenv("CODE_SERVER_PASSWORD")},
		{"DOCKER_HUB_USERNAME", d.DockerHub.Username},
		{"DOCKER_HUB_REPO", d.DockerHub.Repository},
		{"GW_ID", t.Name},
		{"GW_NATS_PUBLISH", fmt.Sprint(deployment.CloudLeafPort)},
		{"GW_NATS_LEAF_USER", deployment.CloudLeafUser},
		{"GW_NATS_LEAF_PASSWORD", c.deploy.LeafPassword(t.Name)},
	}
	for _, k := range t.ForwardEnv {
		kv = append(kv, [2]string{k, os.Getenv(k)})
	}
	var b strings.Builder
	b.WriteString("# Generated by gw-server for cloud target " + t.Name + " - do not edit.\n")
	for _, p := range kv {
		if strings.ContainsAny(p[1], "'\n") {
			return "", fmt.Errorf("%s contains a quote or newline, which can't be passed to the VM's .env", p[0])
		}
		fmt.Fprintf(&b, "%s='%s'\n", p[0], p[1])
	}
	return b.String(), nil
}

// cloudMetadata is the instance metadata a target's VM boots from: the
// startup script, its .env, image and profile. fingerprint is the current
// metadata's, required by GCE when replacing it ("" on create).
func (c *Catalog) cloudMetadata(d *deployment.Deployment, t deployment.CloudTarget, fingerprint string) (map[string]any, error) {
	env, err := c.cloudEnv(d, t)
	if err != nil {
		return nil, err
	}
	md := map[string]any{"items": []map[string]string{
		{"key": "startup-script", "value": cloudStartupScript},
		{"key": "d8z-env", "value": env},
		{"key": "d8z-image", "value": d.Ref(t.Image)},
		{"key": "d8z-profile", "value": t.Profile},
		{"key": "d8z-user", "value": firstNonEmpty(os.Getenv("USER"), "d8z")},
	}}
	if fingerprint != "" {
		md["fingerprint"] = fingerprint
	}
	return md, nil
}

// ---- jobs ----

// cloudTarget resolves a job's target arg against deployment.yaml and opens
// a GCP session.
func (c *Catalog) cloudTarget(ctx context.Context, j *jobRun) (*deployment.Deployment, deployment.CloudTarget, *gcpClient, error) {
	d, _, err := c.deploy.Load()
	if err != nil {
		return nil, deployment.CloudTarget{}, nil, err
	}
	t, ok := d.Target(j.arg("target"))
	if !ok {
		return nil, deployment.CloudTarget{}, nil, fmt.Errorf("no target %q in %s", j.arg("target"), c.deploy.File())
	}
	gc, err := c.newGCP(ctx, d)
	if err != nil {
		return nil, deployment.CloudTarget{}, nil, err
	}
	j.logf("GCP project %s, zone %s, as %s", gc.project, gc.zone, gc.email)
	return d, t, gc, nil
}

// cloudDeploy creates the target's VM - or, if it already exists, hands it
// fresh metadata and reboots it, so it pulls the latest images. Then waits
// for the gateway to answer, over HTTP and over NATS.
func cloudDeploy(ctx context.Context, j *jobRun) error {
	c := j.c
	d, t, gc, err := c.cloudTarget(ctx, j)
	if err != nil {
		return err
	}
	if taken, _ := c.mgr.GatewayNameTaken(ctx, t.Name); taken {
		return fmt.Errorf("a local gateway is already named %q: cloud and local gateways share NATS ids, rename the target", t.Name)
	}
	if err := gc.ensureFirewall(ctx, j, t); err != nil {
		return err
	}
	name := vmName(t.Name)
	in, err := gc.instance(ctx, name)
	if err != nil {
		return err
	}
	if in == nil {
		md, err := c.cloudMetadata(d, t, "")
		if err != nil {
			return err
		}
		j.logf("creating %s (%s, %d GB, Container-Optimized OS) to run %s with profile %s",
			name, t.MachineType, t.DiskGB, d.Ref(t.Image), t.Profile)
		body := map[string]any{
			"name":        name,
			"machineType": "zones/" + gc.zone + "/machineTypes/" + t.MachineType,
			"labels":      map[string]string{"d8z-gateway": t.Name},
			"tags":        map[string]any{"items": []string{firewallName(t.Name)}},
			"metadata":    md,
			"disks": []map[string]any{{
				"boot": true, "autoDelete": true,
				"initializeParams": map[string]any{
					"sourceImage": "projects/cos-cloud/global/images/family/cos-stable",
					"diskSizeGb":  fmt.Sprint(t.DiskGB),
				},
			}},
			"networkInterfaces": []map[string]any{{
				"network":       "global/networks/default",
				"accessConfigs": []map[string]string{{"type": "ONE_TO_ONE_NAT", "name": "External NAT"}},
			}},
		}
		if err := gc.call(ctx, "POST", "/zones/"+gc.zone+"/instances", body); err != nil {
			return err
		}
	} else {
		j.logf("%s exists (%s): updating its metadata and restarting it to pull the latest images", name, in.Status)
		md, err := c.cloudMetadata(d, t, in.Metadata.Fingerprint)
		if err != nil {
			return err
		}
		if err := gc.call(ctx, "POST", gc.instancePath(name)+"/setMetadata", md); err != nil {
			return err
		}
		verb := "reset"
		if in.Status == "TERMINATED" || in.Status == "STOPPED" {
			verb = "start"
		}
		if err := gc.call(ctx, "POST", gc.instancePath(name)+"/"+verb, nil); err != nil {
			return err
		}
	}
	return c.cloudAwait(ctx, j, gc, t)
}

// cloudAwait records the VM's address, then waits until its gateway answers
// HTTP and, once our leaf link to it is up, a NATS ping.
func (c *Catalog) cloudAwait(ctx context.Context, j *jobRun, gc *gcpClient, t deployment.CloudTarget) error {
	in, err := c.cloudRefresh(ctx, j, gc, t)
	if err != nil {
		return err
	}
	addr := in.address()
	if addr == "" {
		return fmt.Errorf("%s has no external address", vmName(t.Name))
	}
	j.logf("waiting for the gateway at http://%s/ (first boot pulls the image: a few minutes) ...", addr)
	deadline := time.Now().Add(10 * time.Minute)
	for {
		req, _ := http.NewRequestWithContext(ctx, "GET", "http://"+addr+"/", nil)
		resp, err := apiclient.HTTPClient.Do(req)
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusOK {
				j.logf("gateway is up (HTTP %d - the console wants your basic-auth login)", resp.StatusCode)
				break
			}
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("gateway didn't answer on http://%s/ within 10 minutes (check the VM's serial console: startup-script output)", addr)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(10 * time.Second):
		}
	}
	if !c.mgr.NatsAvailable() {
		j.logf("NATS isn't running here: skipping the leaf link check")
		return nil
	}
	c.mgr.Reconcile(ctx) // dial its leaf listener now rather than on the next tick
	j.logf("connecting our NATS leaf link to %s:%d and pinging %s ...", addr, deployment.CloudLeafPort, t.Name)
	var reply string
	for i := 0; i < 24; i++ {
		if reply, err = c.mgr.NatsPing(t.Name, "hello from "+c.mgr.GatewayID(), 3*time.Second); err == nil {
			break
		}
		time.Sleep(2 * time.Second)
	}
	if err != nil {
		return fmt.Errorf("gateway is up on HTTP but didn't answer over NATS: %v (is %d open to us in allow_from?)", err, deployment.CloudLeafPort)
	}
	j.logf("NATS ping reply: %s\n\nhttp://%s/", reply, addr)
	return nil
}

// cloudRefresh reads the VM and records its state.
func (c *Catalog) cloudRefresh(ctx context.Context, j *jobRun, gc *gcpClient, t deployment.CloudTarget) (*gceInstance, error) {
	in, err := gc.instance(ctx, vmName(t.Name))
	if err != nil {
		return nil, err
	}
	if in == nil {
		c.deploy.SetState(t.Name, nil)
		return nil, fmt.Errorf("%s doesn't exist (run cloud/deploy)", vmName(t.Name))
	}
	st := deployment.CloudState{Instance: in.Name, Zone: gc.zone, Address: in.address(), Status: in.Status}
	if err := c.deploy.SetState(t.Name, &st); err != nil {
		j.logf("warning: can't save %s: %v", c.deploy.StateFile(), err)
	}
	j.logf("%s: %s, address %s", in.Name, in.Status, firstNonEmpty(st.Address, "(none)"))
	return in, nil
}

// cloudStatus is the cloud/status job: records the VM's state and, if it is
// running, pings its gateway over NATS.
func cloudStatus(ctx context.Context, j *jobRun) error {
	_, t, gc, err := j.c.cloudTarget(ctx, j)
	if err != nil {
		return err
	}
	in, err := j.c.cloudRefresh(ctx, j, gc, t)
	if err != nil {
		return err
	}
	if in.Status == "RUNNING" && j.c.mgr.NatsAvailable() {
		j.c.mgr.Reconcile(ctx)
		if reply, err := j.c.mgr.NatsPing(t.Name, "status", 3*time.Second); err != nil {
			j.logf("NATS: no answer from %s yet (%v)", t.Name, err)
		} else {
			j.logf("NATS ping reply: %s", reply)
		}
	}
	return nil
}

// cloudStart is the cloud/start job: starts the VM and waits for its gateway.
func cloudStart(ctx context.Context, j *jobRun) error {
	_, t, gc, err := j.c.cloudTarget(ctx, j)
	if err != nil {
		return err
	}
	j.logf("starting %s ...", vmName(t.Name))
	if err := gc.call(ctx, "POST", gc.instancePath(vmName(t.Name))+"/start", nil); err != nil {
		return err
	}
	return j.c.cloudAwait(ctx, j, gc, t)
}

// cloudStop is the cloud/stop job: stops the VM (releasing its address) and
// drops our leaf link to it.
func cloudStop(ctx context.Context, j *jobRun) error {
	_, t, gc, err := j.c.cloudTarget(ctx, j)
	if err != nil {
		return err
	}
	j.logf("stopping %s (its external address is released) ...", vmName(t.Name))
	if err := gc.call(ctx, "POST", gc.instancePath(vmName(t.Name))+"/stop", nil); err != nil {
		return err
	}
	_, err = j.c.cloudRefresh(ctx, j, gc, t)
	j.c.mgr.Reconcile(ctx)
	return err
}

// cloudDelete is the cloud/delete job: deletes the VM (with its boot disk)
// and its firewall rule, and forgets its state.
func cloudDelete(ctx context.Context, j *jobRun) error {
	_, t, gc, err := j.c.cloudTarget(ctx, j)
	if err != nil {
		return err
	}
	name := vmName(t.Name)
	if in, err := gc.instance(ctx, name); err != nil {
		return err
	} else if in != nil {
		j.logf("deleting %s (and its boot disk) ...", name)
		if err := gc.call(ctx, "DELETE", gc.instancePath(name), nil); err != nil {
			return err
		}
	} else {
		j.logf("%s doesn't exist", name)
	}
	fw := "/global/firewalls/" + firewallName(t.Name)
	if err := gc.call(ctx, "DELETE", fw, nil); err != nil && !apiclient.IsStatus(err, 404) {
		return err
	}
	j.logf("removed firewall rule %s", firewallName(t.Name))
	j.c.deploy.SetState(t.Name, nil)
	j.c.mgr.Reconcile(ctx)
	return nil
}

// cloudList lists every d8z gateway VM in the project's zone.
func cloudList(ctx context.Context, j *jobRun) error {
	d, _, err := j.c.deploy.Load()
	if err != nil {
		return err
	}
	gc, err := j.c.newGCP(ctx, d)
	if err != nil {
		return err
	}
	var list struct {
		Items []gceInstance `json:"items"`
	}
	q := url.Values{"filter": {"labels.d8z-gateway:*"}}
	if err := gc.do(ctx, "GET", "/zones/"+gc.zone+"/instances?"+q.Encode(), nil, &list); err != nil {
		return err
	}
	j.logf("d8z gateways in %s/%s:", gc.project, gc.zone)
	for _, in := range list.Items {
		j.logf("  %-24s %-11s %-15s target %s", in.Name, in.Status, firstNonEmpty(in.address(), "-"), in.Labels["d8z-gateway"])
	}
	j.logf("%d instances.", len(list.Items))
	return nil
}
