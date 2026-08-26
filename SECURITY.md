# Security Policy

## Supported Versions

| Version | Supported |
| ------- | --------- |
| main    | ✅        |

dreep-go has not yet cut its first stable release; security fixes land on
`main`. Once versioned tags exist, this table will list them.

## Reporting a Vulnerability

**Please do not report security vulnerabilities through public GitHub issues.**

Instead, report them privately via [GitHub Security Advisories](
https://github.com/IndigoSoftwares21/dreep-go/security/advisories/new)
("Report a vulnerability"), or email **security@indigosoftwares.dev**.

Include as much of the following as you can:

* The type of issue and its impact
* Step-by-step instructions or a proof-of-concept to reproduce it
* Affected commit, branch, or tag
* Any suggested fixes

You can expect an initial response within 72 hours. We will keep you informed
of progress toward a fix and announcement, and we credit reporters in the
advisory unless they prefer to remain anonymous.

## Scope Notes

dreep-go is a client library for the Dreep media API. Areas of particular
interest:

* Handling of API keys (must never be logged, leaked into URLs, or included in
  error messages)
* Signed URL generation (`SignedURL`) — signature correctness and timing safety
* Multipart upload streaming — request smuggling / content-length mismatches

Issues in the Dreep service itself should be reported to Dreep, not here.
