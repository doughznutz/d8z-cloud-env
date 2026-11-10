import { Fragment } from "react";
import type { ReactNode } from "react";

/**
 * Shared rendering for the console's recurring shape: an entity is an
 * identity plus config, state, children and actions - whether that's a
 * whole tab, a composed sub-component (Docker Hub, GCP), or a single row
 * (an image, a target, a service). These components render that shape
 * consistently at every depth; the domain data (what goes into `facts`,
 * how a status dot is computed) stays with each caller.
 *
 * Every element these primitives render carries an extra "ds-mark" class
 * (see styles.css) - a visible marker, not permanent styling, so it's
 * obvious which parts of the console have moved onto this module during
 * the Control/Deployment migration. Remove the class (and its CSS rule)
 * once everything has.
 */
const MARK = "ds-mark";

/** One label/value pair surfaced from an entity's config or state. */
export interface Fact {
  label: string;
  value: ReactNode;
  tone?: "muted" | "warn" | "error" | "ok";
}

const TONE_CLASS: Record<NonNullable<Fact["tone"]>, string> = {
  muted: "muted",
  warn: "warn-text",
  error: "error-text",
  ok: "ok-text",
};

/** Label/value grid (`<dl class="facts">`) - config/state display at the page and card tiers. */
export function Facts({ items }: { items: Fact[] }) {
  return (
    <dl className="facts">
      {items.map((f) => (
        <Fragment key={f.label}>
          <dt>{f.label}</dt>
          <dd className={f.tone ? TONE_CLASS[f.tone] : undefined}>{f.value}</dd>
        </Fragment>
      ))}
    </dl>
  );
}

/**
 * The one outer bordered box per route. Carries no entity data - just the
 * chrome every page needs exactly once, so {@link EntityCard} can be used
 * for a page's own identity the same way it's used for a nested entity,
 * without doubling up borders.
 */
export function Panel({ wide, children }: { wide?: boolean; children?: ReactNode }) {
  return <div className={`panel${wide ? " wide" : ""} ${MARK}`}>{children}</div>;
}

/** Props for {@link EntityCard}. */
export interface EntityCardProps {
  title: ReactNode;
  /**
   * "page": this card IS the route's own identity (h2, no dep-head chrome
   * - the page title doesn't need a trailing-actions layout of its own).
   * "card" (default): a composed sub-entity within the page (h3, dep-head
   * chrome, e.g. Docker Hub, GCP).
   */
  level?: "page" | "card";
  tag?: ReactNode;
  actions?: ReactNode;
  description?: ReactNode;
  facts?: Fact[];
  error?: string | null;
  children?: ReactNode;
}

/**
 * A singleton entity: identity + facts + optional nested entities (an
 * inline edit form is just another child, placed wherever the caller
 * wants it relative to the rest). The same component renders a whole
 * page's own identity (inside one {@link Panel}) and a composed
 * sub-entity (Docker Hub, GCP, workspace status, a future NATS messaging
 * card) - they're the same shape, one level apart.
 */
export function EntityCard({ title, level = "card", tag, actions, description, facts, error, children }: EntityCardProps) {
  const Heading = level === "page" ? "h2" : "h3";
  return (
    <>
      <Heading className={[level === "card" ? "dep-head" : null, MARK].filter(Boolean).join(" ")}>
        {title}
        {tag && <small className="muted"> {tag}</small>}
        {actions && <span className={level === "card" ? "dep-head-actions" : undefined}>{actions}</span>}
      </Heading>
      {description && <p className="muted">{description}</p>}
      {facts && facts.length > 0 && <Facts items={facts} />}
      {error && <div className="banner error">{error}</div>}
      {children}
    </>
  );
}

/** Props for {@link EntitySection}. */
export interface EntitySectionProps {
  title: ReactNode;
  /** e.g. "+ Add image", shown next to the title. */
  action?: ReactNode;
  error?: string | null;
  /** Shown instead of the table when there are no children. */
  empty?: ReactNode;
  /** Hides the table body (header, action and error still show) - the caller owns the toggle, usually a button in `action`. */
  collapsed?: boolean;
  children?: ReactNode;
}

/**
 * A list of child entities - Services, Images, Targets, Workspace
 * gateways, a Jobs group. Unlike {@link EntityCard}, it never bundles an
 * add/edit form: the list is a bordered table, and a form attaches below
 * it as the caller's own sibling, not inside the table.
 */
export function EntitySection({ title, action, error, empty, collapsed, children }: EntitySectionProps) {
  const hasChildren = Array.isArray(children) ? children.some(Boolean) : !!children;
  return (
    <>
      <h3 className={[action ? "dep-head" : null, MARK].filter(Boolean).join(" ")}>
        {title}
        {action && <span className="dep-head-actions">{action}</span>}
      </h3>
      {error && <div className="banner error">{error}</div>}
      {collapsed ? null : !hasChildren && empty ? <p className="muted">{empty}</p> : <div className="svc-table">{children}</div>}
    </>
  );
}

/** Props for {@link EntityRow}. */
export interface EntityRowProps {
  /** A CSS dot class ("running" | "degraded" | "error" | ...); omit (or null) to render no status dot. */
  dot?: string | null;
  /** The row's identity, fully caller-formatted (a plain name, a `<code>`, a link - whatever that entity uses elsewhere). */
  id: ReactNode;
  tag?: ReactNode;
  summary?: ReactNode;
  /** "none" renders `.svc-status` unstyled, for a row whose summary carries its own inline tones. */
  summaryTone?: "muted" | "warn" | "error" | "none";
  actions?: ReactNode;
  /** Always-visible secondary content below the head line (e.g. a target's VM status + NATS form). */
  detail?: ReactNode;
  /** Toggle-revealed content (logs, an inline form); the row doesn't own the toggle, the caller does. */
  expanded?: ReactNode;
  open?: boolean;
}

/** One row in an EntitySection's table: a service, gateway, image, target, or job. */
export function EntityRow({ dot, id, tag, summary, summaryTone = "muted", actions, detail, expanded, open }: EntityRowProps) {
  const toneClass = summaryTone === "none" ? "" : TONE_CLASS[summaryTone];
  return (
    <div className={`svc-row ${MARK}${open ? " open" : ""}`}>
      <div className="svc-head">
        {dot != null && <span className={`dot ${dot}`} />}
        {id}
        {tag && <span className="tag">{tag}</span>}
        {summary !== undefined && <span className={`svc-status${toneClass ? ` ${toneClass}` : ""}`}>{summary}</span>}
        {actions}
      </div>
      {detail}
      {expanded}
    </div>
  );
}
