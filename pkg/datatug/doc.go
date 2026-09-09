// Package datatug holds the DataTug project model: entities, queries,
// environments, boards and their validation rules. It is a pure model+engine
// library with no HTTP surface and no concrete DB drivers.
//
// # Query text sidecar file naming
//
// A QueryDef with non-empty Text is persisted as two files under the queries
// folder, both derived from QueryDef.ID and storage.QueryFileSuffix
// ("query"):
//
//   - "<id>.query.json"  - the QueryDef itself (Text stripped), always written.
//   - "<id>.query.<ext>" - the query body, written only when Text is non-empty,
//     where <ext> is strings.ToLower(string(QueryDef.Type)).
//
// This holds for every query type, including DTQL: a DTQL query's body sits
// in "<id>.query.dtql" beside its "<id>.query.json" sidecar, following the
// same convention as "<id>.query.sql" and "<id>.query.http" rather than the
// standalone "<id>.dtql.yaml" name floated in early planning notes - no
// special-casing was needed since pkg/storage/filestore already derives the
// extension generically from QueryDef.Type. The body itself is expected to be
// the YAML produced by DALgo's dtql package (see REQ:dtql-query-type in
// datatug/datatug's core-investigation-loop feature); the ".dtql" suffix
// labels the query type, not the serialization format, matching how ".sql"
// and ".http" already behave.
package datatug
