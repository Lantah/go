---
title: Data
replacement: https://developers.stellar.org/api/resources/accounts/data/
---

Each account in Lantah Network can contain multiple key/value pairs associated with it. Horizon can be used to retrieve value of each data key.

When horizon returns information about a single account data key it uses the following format:

## Attributes

| Attribute | Type | | 
| --- | --- | --- |
| value | base64-encoded string | The base64-encoded value for the key |

## Example

```json
{
  "value": "MTAw"
}
```
