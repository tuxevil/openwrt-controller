# Device Enrollment

Secure provisioning uses two credentials with different lifetimes:

- The site enrollment token is short-lived and is used to fetch the first signed artifact and enroll a new device.
- The device token is issued by enrollment and is required for telemetry, config pulls, threat-list pulls, and agent updates.

## Issue A Token

With an authenticated administrator session and the target tenant selected:

```bash
curl -X POST \
  -H "Authorization: Bearer $JWT" \
  -H "X-Tenant-Schema: tenant_example" \
  http://controller:3000/api/sites/$SITE_ID/enrollment-token
```

The response contains the plaintext token once and its expiration. Store it only in the image build secret material. A subsequent `POST` rotates the token; `DELETE` revokes it. Enrollment requires `auto_adopt=true` on the site.

## Enroll

The bootstrap agent sends the token in `X-Site-Enrollment-Token`:

```http
POST /api/device-enrollment
X-Site-Enrollment-Token: <site-enrollment-token>
Content-Type: application/json

{
  "device_id": "04:A1:51:96:A6:4D",
  "nonce": "0123456789abcdef0123456789abcdef",
  "capabilities": {
    "device_change_set": {
      "version": 1,
      "namespaces": ["system"],
      "max_operations": 1,
      "confirmation_policies": ["local_auto"]
    },
    "architecture": "ath79",
    "kernel": "6.6"
  }
}
```

The controller hashes the site token, persists the device and capabilities, generates a device token, and records the nonce. Retrying the same request with the same nonce returns the same device token; reusing that nonce for another device is rejected. Telemetry never creates unknown devices.

After a successful response the agent deletes the enrollment token and nonce files. If the response is lost, the persisted nonce allows a retry without issuing a second device identity. The enrollment token is not accepted for established-device config, telemetry, or threat-list requests; those use the device token. It may be used to fetch the first signed artifact before enrollment completes.
