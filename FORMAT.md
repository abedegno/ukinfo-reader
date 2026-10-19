# Database formats

The reader supports three source format families across four verified distributions.
Identification uses exact source-file profiles; sharing a product name does not imply
that another release has the same layout.

| Source family | Verified distribution | Reference |
| --- | --- | --- |
| Early SE | PC Format 78 · January 1998 | [Binary layout, index evidence and source defects](docs/formats/pcf78.md) |
| SE 4.0 | PC Format 94 · April 1999 | [Compressed columns, fields and decoding rules](docs/formats/se4.md) |
| Modern SE | PC Plus 218 · July 2004; PC Format 175 · June 2005 | [Address hierarchy, residential/business records and updates](docs/formats/modern-se.md) |

The derived [PostgreSQL catalogue](docs/formats/catalogue.md) is the project's search
representation, separate from the original disc formats. It retains decoder output
alongside indexed search projections.

- [Supported editions, source media and fingerprints](docs/editions.md)
- [Validation overview and complete reports](docs/validation-overview.md)
- [Adding support for another edition](docs/adding-editions.md)
- [Documentation index](docs/README.md)
