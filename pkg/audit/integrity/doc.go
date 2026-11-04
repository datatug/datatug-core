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
//     ExpireAt (the Firestore TTL field) is not part of an event.
//   - Stored events and erasure. A [StoredEvent] is what the store keeps: the
//     event, its salt and the commitments of erased fields.
//     [StoredEvent.Erase] drops a stored value (for example a free-text
//     reason naming a person) and keeps its commitment, so [StoredEvent.Leaf]
//     and [StoredEvent.Disclose] still produce the sealed leaf.
//   - Leaf order. Firestore has no insertion order, so the order is defined
//     here: [OrderLeaves] sorts a window's events by [CompareEvents] (at,
//     then id; the leaf hash breaks a full tie). Sealer and exporter both
//     call it, so leaf positions agree.
//   - Windows. Events fall into 5-minute windows per Space and project
//     ([WindowID]) by arrival time. A window's Merkle tree ([Tree],
//     RFC 9162 shape) is built over its ordered leaves; an empty window has
//     the root of the empty tree. The window root ([RootStatement.Root])
//     commits to the window identity, the previous window's root, the Merkle
//     root, the leaf count and the retention expiry, so the windows form a
//     chain and each window is signed once. A window is sealed only once it
//     is [WindowID.Closed]: its end plus [SealGrace], so an event stamped at
//     the last instant but committed late still lands before sealing.
//   - Signed statements. [Signer] and [Verifier] abstract the signing key
//     (production: a Cloud KMS key held by the platform operator; tests and
//     local tools: [Ed25519Signer], [Ed25519Verifier]). [Sign] produces a
//     [Signed] statement for window roots ([RootStatement], which carries the
//     expiry, so a later absence is pre-explained by the same signature),
//     retention tombstones ([TombstoneStatement], never dated before the
//     signed expiry) and legal-hold place/release records ([HoldStatement];
//     a release names two distinct people and only lifts a placement of the
//     same hold and range made no later than the release).
//   - Verification. [VerifyExport] checks an [Export]: signatures, the chain
//     of previous-root links, contiguity of the claimed window range,
//     optional independent anchors (roots in [VerifyOptions.Anchors], holds
//     in [VerifyOptions.AnchoredHolds]), tombstones against legal holds, and
//     every included event's commitments, inclusion proof, canonical order
//     and revealed location (window, Space, project). The [Report] says
//     exactly what is proven: every included event is authentic; a window is
//     complete when every event (leaf) is included, and disclosed when no
//     included event hides a field. Hidden fields are listed as "redacted"
//     or "erased" by the exporter's unauthenticated label, so neither label
//     changes a verdict. Hold coverage is independent only when the anchored
//     hold set was supplied.
//     Failures are a [*VerifyError] naming the first broken window, matching
//     one sentinel error per tamper class (see errors.go).
//   - Pseudonyms. [NewActorRef] makes random pseudonymous actor ids;
//     [IdentityMap] is the reference (in-memory) shape of the erasable
//     pseudonym → user map; [VerifyErasure] checks that an export still
//     verifies and shows no trace of an erased user id.
//
// # How the other layers call it
//
//   - Backend writer (request path): pick [WindowFor](space, project, arrival
//     time), map each person to an [ActorRef] through the identity map, draw a
//     [NewSalt], and insert the event with its salt. Nothing is hashed or
//     signed on the request path.
//   - Erasure: delete the identity-map entry; for free text that names the
//     person, call [StoredEvent.Erase] on the stored record and write back the
//     event and its Erased commitments.
//   - Sealer task (Cloud Tasks, sealing service account): for each unsealed
//     window that is [WindowID.Closed], in order, read its stored events, call
//     [OrderLeaves], [Seal] with the previous root and the retention expiry
//     (at least [RetentionFloor] after the window ends),
//     [Sign] the [RootStatement] with the KMS-backed [Signer] (one signature
//     per window) and write it to the anchor bucket. Setting expireAt on the
//     events is a separate write (see the spec note on Firestore access). A
//     retention deletion is recorded by a signed [TombstoneStatement]
//     carrying the window's seal; a legal hold by signed [HoldStatement]s in
//     the anchor bucket.
//   - Exporter (audit/exports route): call [OrderLeaves] on the window's
//     stored events, build one [NewTree], [StoredEvent.Disclose] the selected
//     events, apply reader redactions with [DisclosedEvent.Redact], attach
//     [Tree.Proof]s and the signed statements as an [Export] (JSON tags are
//     stable).
//   - CLI verifier (`datatug access audit verify <export>`): decode the
//     [Export], build an [Ed25519Verifier] (or a KMS public-key verifier) from
//     the operator's public key, pass roots and the complete hold set read
//     from the anchor bucket in [VerifyOptions], and print the [Report].
//
// # Future option: signed checkpoints
//
// An export carries every window root in its range: about 105,000 per
// project per year, mostly empty windows. A later version could sign a
// checkpoint every N windows (a Merkle root over those window roots), so an
// export carries the checkpoints plus only the windows that hold included
// events, with a proof of each window root in its checkpoint. Not
// implemented; it would add a statement kind, not change the ones here.
//
// # Residual risks
//
//   - Integrity starts at sealing (permission model §19.4). This package
//     cannot detect changes made to a window before it was sealed.
//   - A tombstone's DeletedAt is asserted by the operator's signer. Firestore
//     TTL purges up to about a day after expireAt, so a hold placed after
//     expiry but before the actual purge can be bypassed by backdating
//     DeletedAt to the expiry. Closing this needs an independent deletion
//     time, such as the anchor bucket's object creation time for the
//     tombstone, compared against the holds.
//   - A real-key re-signing of the last window, the first window's link, and
//     hold coverage are checked independently only when the caller passes
//     anchored roots and holds ([VerifyOptions]).
package integrity
