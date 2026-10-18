# Exact-address exploration validation

18 October 2026. Builds on the existing name/postcode projection and lossless source
blocks. No extra source-data copy, premises index or full-catalogue rebuild was needed.
See [user/API behaviour and matching rules](../address-exploration.md).

## Correctness checks

Invented PostgreSQL integration fixtures cover:

- More than 300 co-listed occurrences, mixed people/business listings, and paging.
- Different house numbers, named buildings, flats and missing subbuilding components.
- Raw-only/incomplete postcodes, source warnings, unsupported field types and numeric
  blank house identifiers, which must not be inferred into precise premises.
- Edition/date ordering independently of dataset import order; complete-edition and
  standalone-sample scope, with redundant samples excluded from all-edition views.
- More than two initial postcode batches without matching premises, followed by late
  matches. Paging continues through these batches rather than reporting a false end.
- Invalid/changed record references, cross-address/scope cursor rejection and lossless
  large-integer values in record/address NDJSON exports.

Browser integration covers direct record links, copied source guards, same-address
views, edition scope, pagination, reload, browser Back, return-to-address navigation,
raw exports, unavailable-premises explanations and mobile width. Saved links from a
foreign/old origin resolve locally; the pasted origin is not fetched or navigated to.
The packaged Electron check imports invented records and exercises address exploration
and a saved reference containing an obsolete local server address.

## Existing full-catalogue sample

A bounded postcode-district query selected one eligible person anchor from each of the
four complete editions, plus a 2005 business anchor. For each anchor, the reference
exhausted ordinary exact-postcode person **and** business queries across the selected
edition set, then independently recomputed the conservative display-field premises key
in Python. It compared those occurrence sets with `/api/address` in both edition and
all-edition scope at page size two, checking duplicate-free cursor traversal.

All **10 scoped walks** agreed with the exhaustive postcode reference. **31 returned
occurrences** were checked across those walks; anchors overlap, so this is not a count
of distinct records or people. Sampled page timings were **3.93–29.18 ms** locally.
The [aggregate results](premises-validation-results.json) contain counts/timings only;
no anchor IDs, names, premises fields or original records are retained in the report.

PostgreSQL 18.6 was capped at two CPUs and 1 GiB RAM in Docker; Go ran natively on a
macOS ARM64 host outside that cap. These warm, narrow sample checks are not a hosted
capacity test or a whole-catalogue proof. Broad/pathological postcode groups remain
subject to request deadlines; matching fails explicitly instead of truncating results.

Repeat with your own indexed source catalogue:

```sh
python tools/benchmark_premises.py --url http://127.0.0.1:58318 \
  --output private/address-index/premises-validation.json
```

The tool selects temporary anchors internally, writes only aggregate results and
performs no database mutation. The unit/browser fixtures are invented and run in CI.
