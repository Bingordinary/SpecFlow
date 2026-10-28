```yaml
acceptance_item_set:
  - id: auth.login_success
    promise: auth.login_api
    source: [Authentication model]
    description: Given valid credentials, when login is called, then 200 + token is returned.
    verification_type: testable
    verification_surface: api
    implementation_surface: auth/login.go
    verification_method: integration test for login success.
    pass_condition: The login success scenario passes.
    runnable: yes
    evidence_requirements: [integration_test_pass]
    affects:
      files: [auth/login.go]
      evidence_files: [auth/login_test.go]
```
