---
name: api-doc
description: Update OpenAPI spec from gateway handler code and validate consistency
---

# Update API Documentation

Sync the OpenAPI spec with the current gateway handler code.

## Files

- **Spec**: `pkg/api/openapi.yaml` (OpenAPI 3.0.3)
- **Handlers**: `pkg/api/handler.go`, `pkg/api/state_ws.go`, `pkg/api/config_api.go`
- **Swagger UI**: `pkg/api/swagger.go`

## Steps

1. Read all handler files in `pkg/api/` to identify current endpoints
2. Read `pkg/api/openapi.yaml` to understand current spec
3. Compare endpoints: find any handlers not documented in the spec, or spec entries with no handler
4. For missing endpoints, add them to `openapi.yaml` following the existing style:
   - Use `$ref` for shared schemas in `components/schemas`
   - Include request/response examples where helpful
   - Add proper error responses (400, 401, 404, 500)
5. For removed endpoints, remove from spec
6. Validate the YAML is well-formed
7. Run `make check` to verify nothing is broken

## Usage

`/api-doc` — Full sync of spec with handlers
`/api-doc pkg/api/config_api.go` — Update spec for a specific handler file
