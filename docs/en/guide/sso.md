# SSO Login

You will learn how to configure GitHub OAuth or a standard OIDC login source for DuShengCDN, how to set callback URLs, and how third-party accounts bind to local users.

DuShengCDN supports third-party login through authentication sources. The current supported source types are GitHub OAuth and standard OIDC providers, such as Logto, authentik, Keycloak, and Casdoor.

After an authentication source is configured and enabled, it appears on the login page. Users can sign in with the third-party account or bind it to the current local account while already signed in.

## Before You Start

Prepare:

| Item | Description |
| --- | --- |
| DuShengCDN public URL | The URL users open in their browser, such as `https://dushengcdn.example.com` |
| Source name | Internal unique name, such as `github` or `company-oidc` |
| Client ID | Provided by the third-party application |
| Client Secret | Provided by the third-party application |
| OIDC Discovery URL | Required only for OIDC, such as `https://idp.example.com/.well-known/openid-configuration` |

Confirm that the server address in system settings matches the domain users access.

The source name can contain letters, numbers, hyphens, and underscores, and must start with a letter or number. The source name is part of the callback URL. If you rename it later, update the callback URL in the third-party platform too.

## Callback URL

Set the Redirect URI / Callback URL in the third-party platform to:

```text
<DuShengCDN public URL>/oauth/<source name>
```

Examples:

```text
https://dushengcdn.example.com/oauth/github
https://dushengcdn.example.com/oauth/company-oidc
```

When creating or editing an authentication source, the UI shows the callback URL based on the current browser URL and source name.

## Configure GitHub Login

1. Create an OAuth App in GitHub.
2. Set `Homepage URL` to the DuShengCDN public URL.
3. Set `Authorization callback URL` to the callback shown by DuShengCDN, such as `https://dushengcdn.example.com/oauth/github`.
4. Copy the Client ID and Client Secret.
5. Sign in to DuShengCDN and open Settings -> System Settings -> Authentication Sources.
6. Add a source and select `GitHub`.
7. Fill in source name, display name, Client ID, and Client Secret.
8. Keep the default scope `user:email` unless your GitHub app requires a different value.
9. Save and enable the source.

The login page will show the GitHub button after the source is enabled.

## Configure OIDC Login

1. Create an application or client in the OIDC provider.
2. Choose a Web / Confidential Client type.
3. Set Redirect URI / Callback URL to the value shown by DuShengCDN, such as `https://dushengcdn.example.com/oauth/company-oidc`.
4. Copy the Client ID and Client Secret.
5. Get the provider Discovery URL, usually ending in `/.well-known/openid-configuration`.
6. Sign in to DuShengCDN and open Settings -> System Settings -> Authentication Sources.
7. Add a source and select `OIDC`.
8. Fill in source name, display name, Client ID, Client Secret, and OIDC Discovery URL.
9. Keep the default scope `openid profile email` unless the provider restricts scopes.
10. Save and enable the source.

The login page will show the OIDC button after the source is enabled.

## Login and Binding Behavior

| Scenario | Behavior |
| --- | --- |
| Third-party account already bound to a local user | Sign in directly |
| User is already signed in and starts third-party authorization | Bind the third-party account to the current local user |
| Third-party account is unbound and registration is allowed | Create a normal local user and bind it |
| Third-party account is unbound and registration is disabled | Ask the user to enter existing local credentials to bind |

If you only want existing users to use SSO, disable registration. Unbound third-party accounts will enter the existing-account binding flow.

## Update a Source

When editing an authentication source, leave Client Secret empty to keep the existing secret. Entering a new value overwrites it.

If you change the source name, the callback URL changes too. Update Redirect URI / Callback URL in the third-party platform, or the provider will reject the callback.

## FAQ

### `invalid_scope`

The provider does not allow the configured scope. The OIDC default is `openid profile email`; the GitHub default is `user:email`. Adjust the scope in DuShengCDN or allow it in the provider.

### Callback URL Mismatch

Check that the Redirect URI / Callback URL in the provider exactly matches the URL shown by DuShengCDN. Protocol, domain, port, and path must all match.

### No Third-Party Login Button

Check that the source is enabled and that Client ID and Client Secret are saved. DuShengCDN validates these fields before enabling a source.

### Client Secret Is Not Shown in the List

This is expected. DuShengCDN does not return Client Secret through the API; it only shows whether the secret is configured.
