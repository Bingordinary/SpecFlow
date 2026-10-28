```yaml
promises:
  - id: auth.login_api
    summary: Login returns a token on success and 401 on invalid credentials.
    source: [Authentication model]
    contract:
      operations:
        login:
          inputs:
            username: {type: string, required: true}
            password: {type: string, required: true}
          outputs:
            token: {type: string, required: true}
          errors:
            401: INVALID_CREDENTIALS
          timing: sync
    depends_on: [common.error_schema]
```
