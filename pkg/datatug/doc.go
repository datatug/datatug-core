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
// A TugQL query has one persisted type, "TugQL", and two body representations
// told apart by the body file's name alone (never by sniffing the content):
// TugQL text in "<id>.query.tugql", or - when QueryDef.StructuredBody is set -
// the structured YAML document produced by DALgo's tugql package in
// "<id>.query.tugql.yaml". Having both files for one query is an error.
// Projects written before the TugQL cutover are still read by the file
// store's legacy block (removed in TugQL cutover step 9); a save always writes
// the new forms.
package datatug
