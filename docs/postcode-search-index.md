# Postcode-only search index

The candidate server uses a covering B-tree in the existing ParadeDB projection
for postcode-only searches ordered by name, for people and businesses. Searches
with names, address refinements, nearby criteria or grouping retain their existing
routes. Postcode precision, source hydration, cursor binding and cross-edition
ordering remain shared with the existing search code.

This path requires a complete, valid generation-matched index on every selected
edition. The handler checks its keys, included columns, canonical C collations,
default B-tree operator classes, ordering, owner and table identity. Missing or
drifted indexes fail closed; the handler never creates indexes or retries the
request against the source database. `paradedb_postcode_scalar` is a bounded plan
metric; integration tests also inspect executed candidate SQL, so a metric alone
cannot mask routing changes after cursor catalogue binding.

`BuildParadeIndex` creates the covering index before publishing a new projection
as ready. Running the existing index builder against an already-ready projection
adds a missing index concurrently, without recopying records. Exact existing
indexes are reused; invalid or differently defined same-name objects require
operator investigation. The compatibility name is
`p8_pc_<generation-table>_cover`, retained to reuse the verified prototype.
Neither the source generation schema nor its version changes.

Use the existing `ukinfo-parade-index` management command with its explicit source
reader, target owner, existing reader role, target filesystem checks and dataset
scope. Keep credentials in the existing secret configuration. Hosted provisioning
must use the reviewed NAS controller to enforce serial creation, maintenance
limits, lifecycle guards, private evidence and independent cleanup. Do not deploy
a server requiring these indexes before provisioning all selected generations.

The candidate implementation is not yet qualified or live on the hosted archive.
One prototype's generic index-only plan and warm timings are verified, but the
original full reference, cursor, load, cold and publication gates remain required.
The earlier failed public reference check remains recorded in homelab.
