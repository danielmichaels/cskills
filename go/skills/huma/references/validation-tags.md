# Huma Validation Tags Reference

Complete list of struct tags for request validation and OpenAPI schema generation.

## String Validation

| Tag | Purpose | Example |
|-----|---------|---------|
| `minLength:"N"` | Minimum string length | `minLength:"1"` |
| `maxLength:"N"` | Maximum string length | `maxLength:"255"` |
| `pattern:"regex"` | Regex pattern | `pattern:"^[a-z]+$"` |
| `patternDescription:"text"` | Human-readable pattern error | `patternDescription:"lowercase letters only"` |
| `format:"fmt"` | Format hint | `format:"email"` |

## Number Validation

| Tag | Purpose | Example |
|-----|---------|---------|
| `minimum:"N"` | Inclusive minimum | `minimum:"0"` |
| `exclusiveMinimum:"N"` | Exclusive minimum | `exclusiveMinimum:"0"` |
| `maximum:"N"` | Inclusive maximum | `maximum:"100"` |
| `exclusiveMaximum:"N"` | Exclusive maximum | `exclusiveMaximum:"101"` |
| `multipleOf:"N"` | Must be divisible by N | `multipleOf:"2"` |

## Array Validation

| Tag | Purpose | Example |
|-----|---------|---------|
| `minItems:"N"` | Minimum array length | `minItems:"1"` |
| `maxItems:"N"` | Maximum array length | `maxItems:"50"` |
| `uniqueItems:"true"` | All items must be unique | `uniqueItems:"true"` |

## Object Validation

| Tag | Purpose | Example |
|-----|---------|---------|
| `minProperties:"N"` | Minimum number of keys | `minProperties:"1"` |
| `maxProperties:"N"` | Maximum number of keys | `maxProperties:"20"` |
| `additionalProperties:"true"` | Allow unknown fields | On dummy `_` field |

## General Tags

| Tag | Purpose | Example |
|-----|---------|---------|
| `required:"true"` | Field must be present | `required:"true"` |
| `enum:"a,b,c"` | Allowed values (comma-separated) | `enum:"draft,published,archived"` |
| `default:"val"` | Default value if not provided | `default:"draft"` |
| `nullable:"true"` | Allow JSON null | On dummy `_` field for structs |
| `readOnly:"true"` | Response only (excluded from request schema) | `readOnly:"true"` |
| `writeOnly:"true"` | Request only (excluded from response schema) | `writeOnly:"true"` |
| `deprecated:"true"` | Mark field as deprecated | `deprecated:"true"` |
| `dependentRequired:"a,b"` | If this field is set, a and b are also required | `dependentRequired:"city,state"` |

## Documentation Tags

| Tag | Purpose | Example |
|-----|---------|---------|
| `doc:"text"` | Field description in OpenAPI | `doc:"User's email address"` |
| `example:"val"` | Example value in OpenAPI | `example:"user@example.com"` |
| `hidden:"true"` | Exclude from generated docs | `hidden:"true"` |

## Supported Format Values

`date-time`, `date-time-http`, `date`, `time`, `duration`, `email`, `idn-email`, `hostname`, `ip`, `ipv4`, `ipv6`, `uri`, `iri`, `uri-reference`, `iri-reference`, `uri-template`, `json-pointer`, `relative-json-pointer`, `regex`, `uuid`

## Nullability Rules

- Pointers to scalars (`*string`, `*int`, `*bool`) are nullable by default unless `omitempty` is set
- Struct types are never nullable by default
- To make an entire struct nullable, add `nullable:"true"` on a dummy `_` field:

```go
type MyStruct struct {
    _ struct{} `nullable:"true"`
    Name string `json:"name"`
}
```

## Strictness Defaults

- Unknown JSON body fields are **rejected** (`additionalProperties: false`)
- Unknown query parameters are **ignored**
- Change globally: `config.AllowAdditionalPropertiesByDefault = true`
- Reject unknown query params: `config.RejectUnknownQueryParameters = true`