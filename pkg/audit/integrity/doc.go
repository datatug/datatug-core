// Package integrity is the cryptographic core of DataTug Perimeter audit
// (permission model §19.4, architecture §12, API contract §3.4). It is pure:
// no storage, no network, no clock. Every time it uses is passed in.
//
// # What it provides
//
//   - Event leaves. An [Event] is encoded field by field in a fixed, versioned
//     order ([EncodingVersion], [FieldNames]). Each field gets a salted
//     commitment: HMAC-SHA-256 keyed by a per-field salt derived from the
//     event's random [Salt]. The leaf is a hash over all commitments, so a
//     field can be redacted by keeping only its commitment ([DisclosedEvent.Redact])
//     and the leaf still verifies. Disclosing one field reveals only that
//     field's salt, never the event salt, so redacted values stay hidden.
//     ExpireAt (the Firestore TTL field) is not part of an event: it is set at
//     sealing and excluded from commitments.
//   - Windows. Events fall into 5-minute windows per Space and project
//     ([WindowID]). A window's Merkle tree ([MerkleRoot], RFC 9162 shape) is
//     built over its leaves in insertion order; an empty window has the root
//     of the empty tree. The window root ([RootStatement.Root]) commits to the
//     window identity, the previous window's root, the Merkle root and the
//     leaf count, so the windows form a chain. [InclusionProof] and
//     [VerifyInclusion] prove one leaf.
//   - Signed statements. [Signer] and [Verifier] abstract the signing key
//     (production: a Cloud KMS key held by the platform operator; tests and
//     local tools: [Ed25519Signer], [Ed25519Verifier]). [Sign] produces a
//     [Signed] statement for window roots ([RootStatement]), expiry statements
//     written at sealing ([ExpiryStatement]), retention tombstones
//     ([TombstoneStatement]) and legal-hold place/release records
//     ([HoldStatement]; a release must name two distinct people).
//   - Verification. [VerifyExport] checks an [Export]: signatures, the chain
//     of previous-root links, contiguity of the claimed window range,
//     optional independent anchors, tombstones, and every included event's
//     commitments and inclusion proof. The [Report] says exactly what is
//     proven: every included event is authentic; a window is complete only
//     when it was exported whole and unredacted. Failures are a [*VerifyError]
//     naming the first broken window, matching one sentinel error per tamper
//     class (see errors.go).
//   - Pseudonyms. [NewActorRef] makes random pseudonymous actor ids;
//     [IdentityMap] is the reference (in-memory) shape of the erasable
//     pseudonym → user map; [VerifyErasure] proves an export still verifies
//     and holds no trace of an erased user id.
//
// # How the other layers call it
//
//   - Backend writer (request path): pick [WindowFor](space, project, arrival
//     time), map each person to an [ActorRef] through the identity map, draw a
//     [NewSalt], and insert the event together with its salt. Nothing is
//     hashed or signed on the request path.
//   - Sealer task (Cloud Tasks, sealing service account): for each unsealed
//     window in order, read its events in insertion order, compute
//     [LeafOf] for each, [Seal] the window with the previous root, [Sign] the
//     [RootStatement] and an [ExpiryStatement] with the KMS-backed [Signer],
//     write both to the anchor bucket, then set expireAt on the events (unless
//     a legal hold covers the window). A retention deletion writes a signed
//     [TombstoneStatement] carrying the window's seal.
//   - Exporter (audit/exports route): [Disclose] each selected event, apply
//     reader redactions with [DisclosedEvent.Redact], attach
//     [InclusionProof]s computed from the window's full leaf list, and ship
//     the signed statements as an [Export] (JSON tags are stable).
//   - CLI verifier (`datatug access audit verify <export>`): decode the
//     [Export], build an [Ed25519Verifier] (or a KMS public-key verifier) from
//     the operator's public key, pass any independently anchored roots in
//     [VerifyOptions.Anchors], and print the [Report].
//
// Residual risk (permission model §19.4): integrity starts at sealing. This
// package cannot detect changes made to a window before it was sealed.
package integrity
