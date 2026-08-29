# Reproducible Linux Meta Reader Implementation Plan

1. Restore the reviewed Meta client byte-for-byte after verifying its SHA-256, and narrow the root account-data ignore rule without exposing private data.
2. Add focused tests first for access-token redaction and bounded GET pagination, confirm they fail before the client is restored, then make them pass.
3. Add a least-privilege `adform-reader` entry point that accepts only the required `stats` command and rejects all writer or unknown surfaces; cover its command boundary with tests.
4. Add deterministic static Linux/amd64 build tooling using `-trimpath`, `-buildvcs=false`, and an empty build ID, emitting an auditable SHA-256 checksum.
5. Run formatting, the full test suite, vet, and two independent builds; compare their bytes and record the verified artifact checksum.
