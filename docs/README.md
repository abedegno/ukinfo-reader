# Documentation

UK-Info Archive searches four verified historical UK-Info editions. Choose a guide
for your task; dated investigation reports describe the evidence at the time.

## Start here

| Task | Guide |
| --- | --- |
| Search without installing | [Public archive](https://ukinfo.6153112.xyz) |
| Set up the desktop app | [First-launch setup](desktop-setup.md) · [Platforms, builds and release status](desktop-releases.md) |
| Run a web service or development instance | [Search application](search-app.md) · [Hosting and security controls](hosting.md) |
| Read original files with Python | [Quick start](reader-quick-start.md) · [CLI reference](usage.md) · [Python API](api.md) |
| Identify a disc or find source media | [Supported editions and fingerprints](editions.md) |

## Search features

- [Search form and matching controls](search-interface.md)
- [Names, middle initials and premises filters](search-refinements.md)
- [Street, locality, town and county search](address-search.md)
- [Grouping, ordering, source references and business classifications](research-index.md)
- [Record links and exact-address exploration](address-exploration.md)
- [Research lists, notes, saved searches and comparison](research-lists.md)
- [Postcode reference and nearby search](geography.md)
- [Optional maps and position accuracy](maps.md)

## Manage and operate an archive

- [Catalogue inventory, removal, backup and restore](catalogue-management.md)
- [OpenTelemetry and Splunk backend setup](observability.md)
- [Desktop diagnostics and safe issue reports](diagnostics.md)
- [Desktop packaging and release gates](desktop-releases.md)
- [macOS signing and notarization](macos-signing.md)
- [Desktop candidate release notes](desktop-release-notes.md)
- [Security reporting and supported builds](../SECURITY.md)

## Formats and preservation

- [Binary format reference](../FORMAT.md)
- [1998: PC Format 78](formats/pcf78.md)
- [1999: SE 4.0](formats/se4.md)
- [2004/2005: address joins, residential/business updates and source anomalies](formats/modern-se.md)
- [PostgreSQL catalogue representation](formats/catalogue.md)
- [Validation overview and complete source reports](validation-overview.md)
- [Adding support for another edition](adding-editions.md)

## Development and research

[Contributing](../CONTRIBUTING.md) covers setup and verification.
[Changelog](../CHANGELOG.md) records implementation changes. The following are dated
research and measurement records, rather than promises about every deployment:

- [Research index](research/README.md)
- [Complete-edition storage and local performance](research/compact-catalogue.md)
- [Feature validation, index storage and national-query limits](research/review-validation.md)
- [Product review](research/product-review-2026-10-19.md) · [Implementation follow-up](review-implementation.md)

[Back to the project](../README.md)

- [Search performance checks](search-performance.md) — invented CI matrices, work budgets and private HDD diagnostics.
- [Production deployment, monitoring and recovery](production.md) — separate serving credentials, TLS ingress, backups and isolated restore checks.
