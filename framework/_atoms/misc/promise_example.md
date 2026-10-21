```yaml
promises:
  - id: auth.login_api
    kind: request_response
    summary: Login returns a token on success and 401 on invalid credentials.
    contract:
      request:
        username: {type: string, required: true}
        password: {type: string, required: true}
      response:
        token: {type: string, required: true}
      errors:
        401: INVALID_CREDENTIALS
      timing:
        mode: sync
    depends_on: [common.error_schema]
```
