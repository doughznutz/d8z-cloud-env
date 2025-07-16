# Google Cloud Authentication Setup

This document outlines the steps to create a Google Cloud Platform (GCP) service account and generate a key file, which is necessary for this application to manage GCE instances.

## 1. Navigate to the Service Accounts Page

*   Open the [Google Cloud Console](https://console.cloud.google.com/).
*   In the search bar at the top, type "Service Accounts" and select the "Service Accounts" page, or navigate directly to [IAM & Admin > Service Accounts](https://console.cloud.google.com/iam-admin/serviceaccounts).

## 2. Select Your Project

*   Ensure the correct Google Cloud project is selected in the project dropdown menu at the top of the page.

## 3. Create a New Service Account

1.  Click the **"+ CREATE SERVICE ACCOUNT"** button at the top of the page.
2.  **Service account name:** Enter a descriptive name (e.g., `cloud-instance-manager`).
3.  **Description:** Provide an optional description of what this service account will be used for.
4.  Click **"CREATE AND CONTINUE"**.

## 4. Grant Required Permissions

*   In the "Grant this service account access to project" section, you must assign a role that allows the management of GCE instances.
*   Click the **"Role"** dropdown menu.
*   Search for and select the **"Compute Admin"** role (`roles/compute.admin`). This provides comprehensive permissions for Compute Engine resources.
    *   *Note: For a more secure, production environment, you should create a custom role with only the specific permissions required (e.g., `compute.instances.create`, `compute.instances.start`, `compute.instances.stop`, `compute.instances.delete`, `compute.instances.list`).*
*   Click **"CONTINUE"**.

## 5. Generate the Service Account Key

1.  You can skip the optional "Grant users access to this service account" step. Click **"DONE"**.
2.  You will be returned to the list of service accounts. Locate the account you just created.
3.  Click the three-dot "Actions" menu on the far right of the service account's row and select **"Manage keys"**.
4.  Click the **"ADD KEY"** dropdown and select **"Create new key"**.
5.  Choose **JSON** as the key type.
6.  Click **"CREATE"**.

A JSON key file will be automatically downloaded to your computer.

## 6. Store and Use the Key File

1.  **Move the Key:** Place the downloaded JSON file into the `.keys/` directory within this project's root folder.
2.  **Rename (Optional):** You can rename the file to something more descriptive, like `gcp-service-account.json`.
3.  **Configure Environment:** Update the `GOOGLE_APPLICATION_CREDENTIALS_PATH` variable in your `.env` file to match the path of your key file (e.g., `.keys/gcp-service-account.json`).

---

### **IMPORTANT SECURITY NOTICE**

**Treat this key file as you would a password.** It provides direct access to your Google Cloud resources.

*   **DO NOT** commit this key file to your Git repository.
*   Ensure that your project's `.gitignore` file contains an entry for the `.keys/` directory to prevent accidental commits.
