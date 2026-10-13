package sessioncontext

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/dal-go/dalgo/adapters/dalgo2memory"
	"github.com/dal-go/dalgo/dal"
	"github.com/dal-go/record"
	"github.com/datatug/datatug-core/pkg/apicontract"
)

type testMountedDatabase struct {
	id string
	db dal.DB
}

func (d testMountedDatabase) ID() string { return d.id }
func (d testMountedDatabase) DB() dal.DB { return d.db }

func newTestService(t *testing.T, authorize Authorizer) (*Service, Principal) {
	t.Helper()
	db := dalgo2memory.New(dalgo2memory.FirestoreProfile())
	repo, err := NewDALRepository(testMountedDatabase{id: "db1", db: db}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if authorize == nil {
		authorize = func(context.Context, Access) error { return nil }
	}
	svc, err := NewService(repo, authorize)
	if err != nil {
		t.Fatal(err)
	}
	return svc, Principal{Actor: "alice", DatabaseID: "db1", ProjectID: "project1"}
}

func executeOK(t *testing.T, svc *Service, p Principal, c Command) Receipt {
	t.Helper()
	result, err := svc.Execute(context.Background(), p, c)
	if err != nil {
		t.Fatalf("Execute(%s): %v", c.Action, err)
	}
	return result.Receipt
}

func createTestSession(t *testing.T, svc *Service, p Principal, sessionID, bagID string) Receipt {
	t.Helper()
	scope := &Scope{EnvironmentID: "uat", IdentityDomain: "tenant-a"}
	return executeOK(t, svc, p, Command{OpID: "create-" + sessionID, Action: "createSession", SessionID: sessionID, DatabaseID: p.DatabaseID, ProjectID: p.ProjectID, BagID: bagID, Scope: scope, Title: "private session title"})
}

func TestSessionContextDALGOFlowPinsHistoryAndReportsDashboardProvenance(t *testing.T) {
	svc, principal := newTestService(t, nil)
	created := createTestSession(t, svc, principal, "s1", "b1")
	scope := Scope{EnvironmentID: "uat", IdentityDomain: "tenant-a"}
	put := Command{
		OpID: "entry-r1", Action: "putEntry", SessionID: "s1", ExpectedVersion: created.SessionVersion,
		BagID: "b1", ExpectedBagVersion: 1, EntryID: "e1", ExpectedEntryVersion: 0, RevisionID: "r1",
		Alias: "Order", Scope: &scope, Subject: &Subject{ConceptID: "Order", FieldID: "Order.ID", FieldMappingRef: "mapping-r1"},
		Value: &ValueEnvelope{SchemaVersion: 1, Kind: "scalar", Scalar: valuePtr(apicontract.NewIntegerValue("123"))},
	}
	putReceipt := executeOK(t, svc, principal, put)
	if putReceipt.ResourceVersion != 1 || putReceipt.ResourceID != "e1" {
		t.Fatalf("unexpected entry receipt: %+v", putReceipt)
	}
	assertPrivateFieldsAbsentFromOperationMetadata(t, svc, principal, "entry-r1", "private session title", "123", "Order", "mapping-r1")
	duplicateAlias := put
	duplicateAlias.OpID, duplicateAlias.ExpectedVersion, duplicateAlias.ExpectedBagVersion = "entry-duplicate-alias", putReceipt.SessionVersion, 2
	duplicateAlias.EntryID, duplicateAlias.RevisionID = "e2", "r-e2"
	duplicateAlias.Alias = " order "
	if _, err := svc.Execute(context.Background(), principal, duplicateAlias); !errors.Is(err, ErrInvalid) {
		t.Fatalf("duplicate alias within a bag must be rejected, got %v", err)
	}
	setID := "cs_" + opKey(principal, "create-s1")
	apply := Command{OpID: "apply-r1", Action: "applyContext", SessionID: "s1", ExpectedVersion: putReceipt.SessionVersion, BagID: "b1", ExpectedBagVersion: 2, ExpectedSlotVersion: 0, ExpectedContextSetID: setID, Scope: &scope, Enabled: boolPtr(true), Selection: []EntryRef{{EntryID: "e1", RevisionID: "r1"}}}
	applyReceipt := executeOK(t, svc, principal, apply)

	chat := Command{OpID: "chat-1", Action: "pinInput", SessionID: "s1", ExpectedVersion: applyReceipt.SessionVersion, InputSnapshotID: "input-chat", RunID: "run-chat", Kind: "chat", DefinitionID: "assistant", DefinitionRevision: "assistant-r1", DefinitionDocument: testDefinition("chat"), Scope: &scope, Attachments: []EntryRef{{EntryID: "e1", RevisionID: "r1"}}, Text: "private question", ResolvedParameters: map[string]apicontract.TypedValue{"OrderId": apicontract.NewIntegerValue("123")}}
	chatReceipt := executeOK(t, svc, principal, chat)
	assertPrivateFieldsAbsentFromOperationMetadata(t, svc, principal, "chat-1", "private question", "123")
	if chatReceipt.ResourceID != "run-chat" {
		t.Fatalf("unexpected chat receipt: %+v", chatReceipt)
	}
	chatInput, pinned, err := svc.ReadInput(context.Background(), principal, "input-chat")
	if err != nil {
		t.Fatal(err)
	}
	if len(chatInput.Entries) != 1 || len(pinned) != 1 || pinned[0].Value.Scalar.Str != "123" || pinned[0].Subject.FieldMappingRef != "mapping-r1" {
		t.Fatalf("input did not retain exact value and semantic mapping: input=%+v revisions=%+v", chatInput, pinned)
	}
	if chatInput.ResolvedParameters["OrderId"].Str != "123" || chatInput.ParametersOrigin != "client-observed" {
		t.Fatalf("input did not retain caller-observed parameters with provenance: %+v", chatInput)
	}
	if len(chatInput.DefinitionDocument) == 0 || chatInput.DefinitionDigest == "" {
		t.Fatalf("input did not retain definition and digest: %+v", chatInput)
	}
	if _, err := svc.ReadEntryRevision(context.Background(), principal, "s1", "e1", "r1"); err != nil {
		t.Fatalf("authorized historical revision read failed: %v", err)
	}
	plan := Command{OpID: "chat-plan", Action: "recordRunExecution", SessionID: "s1", ExpectedVersion: chatReceipt.SessionVersion, ExpectedRunVersion: 1, RunID: "run-chat", ExecutionDocument: testQueryDefinition("select * from Orders where id = @OrderId"), ResolvedParameters: map[string]apicontract.TypedValue{"OrderId": apicontract.NewIntegerValue("123")}}
	planReceipt := executeOK(t, svc, principal, plan)
	plannedRun, err := svc.ReadRun(context.Background(), principal, "run-chat")
	if err != nil || plannedRun.ExecutionDigest == "" || plannedRun.Version != 2 || plannedRun.ResolvedParameters["OrderId"].Str != "123" {
		t.Fatalf("generated execution plan was not durably pinned: run=%+v err=%v", plannedRun, err)
	}
	planAgain := plan
	planAgain.OpID, planAgain.ExecutionDocument = "chat-plan-change", testQueryDefinition("select * from Orders")
	if _, err := svc.Execute(context.Background(), principal, planAgain); !errors.Is(err, ErrConflict) {
		t.Fatalf("accepted run execution plan must be immutable: %v", err)
	}
	chatDone := executeOK(t, svc, principal, Command{OpID: "chat-result", Action: "reportClientRun", SessionID: "s1", ExpectedRunVersion: planReceipt.ResourceVersion, RunID: "run-chat", Kind: "chat", RunState: "completed", Text: "There are 123 orders."})
	chatRun, err := svc.ReadRun(context.Background(), principal, "run-chat")
	if err != nil || chatRun.OutcomeOrigin != "client-reported" || chatRun.State != "completed" {
		t.Fatalf("chat outcome should be client-reported: run=%+v err=%v", chatRun, err)
	}
	messages, err := svc.ListMessages(context.Background(), principal, "s1", "", 10)
	if err != nil || len(messages.Items) != 2 {
		t.Fatalf("chat history should contain both durable turns: %+v err=%v", messages, err)
	}
	roles := map[string]string{}
	for _, message := range messages.Items {
		roles[message.Role] = message.Text
	}
	if roles["user"] != "private question" || roles["assistant"] != "There are 123 orders." {
		t.Fatalf("chat history lost a turn or role: %+v", messages.Items)
	}

	// A new revision may update semantic mapping without changing the meaning of the
	// already-pinned chat input.
	update := put
	update.OpID = "entry-r2"
	update.ExpectedVersion = chatDone.SessionVersion
	update.ExpectedBagVersion = 2
	update.ExpectedEntryVersion = 1
	update.RevisionID = "r2"
	update.Subject = &Subject{ConceptID: "Order", FieldID: "Order.ID", FieldMappingRef: "mapping-r2"}
	update.Value = &ValueEnvelope{SchemaVersion: 1, Kind: "scalar", Scalar: valuePtr(apicontract.NewIntegerValue("456"))}
	updateReceipt := executeOK(t, svc, principal, update)
	revisions, err := svc.ListEntryRevisions(context.Background(), principal, "s1", "e1", "", 10)
	if err != nil || len(revisions.Items) != 2 || revisions.Items[0].ID != "r1" || revisions.Items[1].Subject.FieldMappingRef != "mapping-r2" {
		t.Fatalf("entry revision history is incomplete: %+v err=%v", revisions, err)
	}
	_, pinned, err = svc.ReadInput(context.Background(), principal, "input-chat")
	if err != nil || len(pinned) != 1 || pinned[0].Value.Scalar.Str != "123" || pinned[0].Subject.FieldMappingRef != "mapping-r1" {
		t.Fatalf("historical pin changed after entry revision: revisions=%+v err=%v", pinned, err)
	}

	dashboard := Command{OpID: "dash-1", Action: "pinInput", SessionID: "s1", ExpectedVersion: updateReceipt.SessionVersion, InputSnapshotID: "input-dashboard", RunID: "run-dashboard", Kind: "dashboard", DefinitionID: "orders", DefinitionRevision: "orders-r1", DefinitionDocument: testDefinition("dashboard"), Scope: &scope}
	dashboardReceipt := executeOK(t, svc, principal, dashboard)
	stop := Command{OpID: "stop-1", Action: "transitionSession", SessionID: "s1", ExpectedVersion: dashboardReceipt.SessionVersion, Transition: "stop"}
	stopReceipt := executeOK(t, svc, principal, stop)
	// Dashboard execution occurs in the authorized browser worker. Its completion
	// uses run CAS independently of the session version and cannot claim Case proof.
	reported := Command{OpID: "dash-result-1", Action: "reportDashboard", SessionID: "s1", ExpectedVersion: 1, ExpectedRunVersion: 1, RunID: "run-dashboard", RunState: "completed", Widgets: []WidgetOutcome{{ID: "table", State: "completed", ResultRef: "result-1"}}, ResultRefs: []string{"result-1"}}
	dashboardDone := executeOK(t, svc, principal, reported)
	if dashboardDone.SessionVersion != stopReceipt.SessionVersion+1 {
		t.Fatalf("dashboard report did not preserve independent session versioning: %+v", dashboardDone)
	}
	gotRun, err := svc.ReadRun(context.Background(), principal, "run-dashboard")
	if err != nil || gotRun.OutcomeOrigin != "client-reported" || gotRun.State != "completed" {
		t.Fatalf("dashboard report provenance was not explicit: run=%+v err=%v", gotRun, err)
	}
	if _, err := svc.Execute(context.Background(), principal, Command{OpID: "case-fake", Action: "reportDashboard", SessionID: "s1", ExpectedRunVersion: 2, RunID: "run-dashboard", RunState: "completed", CaseID: "case1", DetectiveRunID: "detective1"}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("dashboard outcome must not create Detective evidence, got %v", err)
	}

	runs, err := svc.ListRuns(context.Background(), principal, "s1", "", 1)
	if err != nil || len(runs.Items) != 1 || !runs.HasMore || runs.NextCursor != "run-chat" {
		t.Fatalf("first run history page is incomplete: %+v err=%v", runs, err)
	}
	runs2, err := svc.ListRuns(context.Background(), principal, "s1", runs.NextCursor, 1)
	if err != nil || len(runs2.Items) != 1 || runs2.HasMore || runs2.Items[0].ID != "run-dashboard" {
		t.Fatalf("second run history page is incomplete: %+v err=%v", runs2, err)
	}
	messages, err = svc.ListMessages(context.Background(), principal, "s1", "", 10)
	if err != nil || len(messages.Items) != 2 {
		t.Fatalf("message history is incomplete: %+v err=%v", messages, err)
	}

	start := Command{OpID: "start-1", Action: "transitionSession", SessionID: "s1", ExpectedVersion: dashboardDone.SessionVersion, Transition: "start"}
	started := executeOK(t, svc, principal, start)
	clear := Command{OpID: "clear-1", Action: "clearBag", SessionID: "s1", ExpectedVersion: started.SessionVersion, BagID: "b1", ExpectedBagVersion: 3, ExpectedSlotVersion: 1, Scope: &scope}
	clearReceipt := executeOK(t, svc, principal, clear)
	if _, pinned, err = svc.ReadInput(context.Background(), principal, "input-chat"); err != nil || len(pinned) != 1 || pinned[0].ID != "r1" {
		t.Fatalf("clearing current bag erased pinned history: %+v err=%v", pinned, err)
	}
	snapshot, err := svc.GetSnapshot(context.Background(), principal, "s1")
	if err != nil || len(snapshot.Bags) != 1 || len(snapshot.Bags[0].EntryIDs) != 0 || snapshot.Bags[0].ClearVersion != snapshot.Bags[0].Version || snapshot.Session.Version != clearReceipt.SessionVersion {
		t.Fatalf("clear did not atomically empty selected bag: snapshot=%+v err=%v", snapshot, err)
	}
	if len(snapshot.ContextSet.Slots) != 1 || snapshot.ContextSet.Slots[0].BagID != "" || len(snapshot.ContextSet.Slots[0].Entries) != 0 {
		t.Fatalf("clear did not tombstone only the selected applied slot: %+v", snapshot.ContextSet.Slots)
	}
}

func TestResolvedParametersAreBoundedAndPrivate(t *testing.T) {
	svc, principal := newTestService(t, nil)
	created := createTestSession(t, svc, principal, "params", "params-bag")
	scope := Scope{EnvironmentID: "uat", IdentityDomain: "tenant-a"}
	makePin := func(op string, params map[string]apicontract.TypedValue) Command {
		return Command{OpID: op, Action: "pinInput", SessionID: "params", ExpectedVersion: created.SessionVersion, InputSnapshotID: "input-" + op, RunID: "run-" + op, Kind: "query", DefinitionID: "query", DefinitionRevision: "r1", DefinitionDocument: testDefinition("query"), Scope: &scope, ResolvedParameters: params}
	}
	longName := strings.Repeat("x", 129)
	if _, err := svc.Execute(context.Background(), principal, makePin("bad-name", map[string]apicontract.TypedValue{longName: apicontract.NewStringValue("secret")})); !errors.Is(err, ErrInvalid) {
		t.Fatalf("overlong parameter name accepted: %v", err)
	}
	tooMany := make(map[string]apicontract.TypedValue, 101)
	for i := 0; i < 101; i++ {
		tooMany[fmt.Sprintf("p%d", i)] = apicontract.NewIntegerValue("1")
	}
	if _, err := svc.Execute(context.Background(), principal, makePin("too-many", tooMany)); !errors.Is(err, ErrInvalid) {
		t.Fatalf("more than 100 parameters accepted: %v", err)
	}
	if _, err := svc.Execute(context.Background(), principal, Command{OpID: "wrong-action", Action: "createBag", SessionID: "params", ExpectedVersion: created.SessionVersion, BagID: "x", Scope: &scope, ResolvedParameters: map[string]apicontract.TypedValue{"p": apicontract.NewStringValue("secret")}}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("parameters accepted outside pinInput: %v", err)
	}
	if snapshot, err := svc.GetSnapshot(context.Background(), principal, "params"); err != nil || len(snapshot.Bags) != 1 || snapshot.Session.Version != created.SessionVersion {
		t.Fatalf("rejected parameter pins mutated state: snapshot=%+v err=%v", snapshot, err)
	}
}

func TestDefinitionDocumentIsCanonicalPrivateAndImmutable(t *testing.T) {
	first, firstDigest, err := canonicalDefinitionDocument("query", testQueryDefinition("select 1"))
	if err != nil {
		t.Fatal(err)
	}
	reordered := json.RawMessage(`{ "schemaVersion" : 1, "kind":"query", "queries" : [ { "id":"q1", "queryType":"SQL", "text":"select 1" } ] }`)
	canonical, canonicalDigest, err := canonicalDefinitionDocument("query", reordered)
	if err != nil || firstDigest != canonicalDigest || string(first) != string(canonical) {
		t.Fatalf("equivalent document did not canonicalize deterministically: %s %s err=%v", first, canonical, err)
	}
	changed, changedDigest, err := canonicalDefinitionDocument("query", testQueryDefinition("select 2"))
	if err != nil || changedDigest == firstDigest || bytes.Equal(first, changed) {
		t.Fatalf("changed executable text did not change retained digest: %q %q err=%v", firstDigest, changedDigest, err)
	}
	withCredentials := json.RawMessage(`{"schemaVersion":1,"kind":"query","queries":[{"id":"q1","queryType":"SQL","text":"select 1","apiKey":"secret"}]}`)
	if _, _, err := canonicalDefinitionDocument("query", withCredentials); !errors.Is(err, ErrInvalid) {
		t.Fatalf("credential-bearing document accepted: %v", err)
	}
	if _, _, err := canonicalDefinitionDocument("query", json.RawMessage(strings.Repeat("x", 128*1024+1))); !errors.Is(err, ErrInvalid) {
		t.Fatalf("overlong definition document accepted: %v", err)
	}
}

func TestDashboardReportRequiresRunVersionAndCheckExecutor(t *testing.T) {
	svc, principal := newTestService(t, nil)
	created := createTestSession(t, svc, principal, "s2", "b2")
	scope := Scope{EnvironmentID: "uat", IdentityDomain: "tenant-a"}
	if _, err := svc.Execute(context.Background(), principal, Command{OpID: "client-check-pin", Action: "pinInput", SessionID: "s2", ExpectedVersion: created.SessionVersion, InputSnapshotID: "input-check-client", RunID: "run-check-client", Kind: "check", DefinitionID: "check", DefinitionRevision: "r1", DefinitionDocument: testDefinition("check"), Scope: &scope}); !errors.Is(err, ErrForbidden) {
		t.Fatalf("client cannot pin a server-owned Check: %v", err)
	}
	server := principal
	server.Executor = true
	chat := Command{OpID: "check-pin", Action: "pinInput", SessionID: "s2", ExpectedVersion: created.SessionVersion, InputSnapshotID: "input-check", RunID: "run-check", Kind: "check", DefinitionID: "check", DefinitionRevision: "r1", DefinitionDocument: testDefinition("check"), Scope: &scope}
	if _, err := svc.Execute(context.Background(), server, chat); err != nil {
		t.Fatal(err)
	}
	clientOutcome := Command{OpID: "client-check", Action: "reportDashboard", SessionID: "s2", ExpectedRunVersion: 1, RunID: "run-check", RunState: "completed"}
	if _, err := svc.Execute(context.Background(), principal, clientOutcome); !errors.Is(err, ErrConflict) {
		t.Fatalf("client path must not complete a Check: %v", err)
	}
	if _, err := svc.Execute(context.Background(), principal, Command{OpID: "client-seize", Action: "reportClientRun", SessionID: "s2", ExpectedRunVersion: 1, RunID: "run-check", Kind: "check", RunState: "completed"}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("client cannot seize a server-owned Check run: %v", err)
	}
	if _, err := svc.Execute(context.Background(), principal, Command{OpID: "delete-pending", Action: "transitionSession", SessionID: "s2", ExpectedVersion: 1, Transition: "delete"}); !errors.Is(err, ErrConflict) {
		t.Fatalf("delete must be blocked while accepted work is pending: %v", err)
	}
	complete := Command{OpID: "server-check", Action: "completeRun", SessionID: "s2", ExpectedVersion: 1, ExpectedRunVersion: 1, RunID: "run-check", RunState: "completed", ResultRefs: []string{"result-check"}}
	completeReceipt, err := svc.Execute(context.Background(), server, complete)
	if err != nil {
		t.Fatalf("server executor could not complete Check: %v", err)
	}
	got, err := svc.ReadRun(context.Background(), principal, "run-check")
	if err != nil || got.OutcomeOrigin != "server-executor" {
		t.Fatalf("Check completion provenance was not server-owned: %+v err=%v", got, err)
	}
	if _, err := svc.Execute(context.Background(), principal, Command{OpID: "delete-settled", Action: "transitionSession", SessionID: "s2", ExpectedVersion: completeReceipt.Receipt.SessionVersion, Transition: "delete"}); err != nil {
		t.Fatalf("delete should succeed after accepted work settles: %v", err)
	}
}

func TestChatPinsOnlyExplicitAttachmentsRegardlessOfAppliedContextGate(t *testing.T) {
	svc, principal := newTestService(t, nil)
	created := createTestSession(t, svc, principal, "s4", "b4")
	scope := Scope{EnvironmentID: "uat", IdentityDomain: "tenant-a"}
	put := Command{OpID: "entry-s4", Action: "putEntry", SessionID: "s4", ExpectedVersion: created.SessionVersion, BagID: "b4", ExpectedBagVersion: 1, EntryID: "e4", RevisionID: "r4", Subject: &Subject{ConceptID: "Order", FieldID: "Order.ID", FieldMappingRef: "mapping"}, Value: &ValueEnvelope{SchemaVersion: 1, Kind: "scalar", Scalar: valuePtr(apicontract.NewIntegerValue("4"))}}
	putReceipt := executeOK(t, svc, principal, put)
	initialSetID := "cs_" + opKey(principal, "create-s4")
	apply := Command{OpID: "apply-s4", Action: "applyContext", SessionID: "s4", ExpectedVersion: putReceipt.SessionVersion, ExpectedContextSetID: initialSetID, ExpectedBagVersion: 2, ExpectedSlotVersion: 0, BagID: "b4", Enabled: boolPtr(true), Selection: []EntryRef{{EntryID: "e4", RevisionID: "r4"}}}
	applyReceipt := executeOK(t, svc, principal, apply)
	chatWithoutAttachment := Command{OpID: "chat-empty", Action: "pinInput", SessionID: "s4", ExpectedVersion: applyReceipt.SessionVersion, InputSnapshotID: "input-empty", RunID: "run-empty", Kind: "chat", DefinitionID: "assistant", DefinitionRevision: "r1", DefinitionDocument: testDefinition("chat"), Scope: &scope}
	executeOK(t, svc, principal, chatWithoutAttachment)
	input, _, err := svc.ReadInput(context.Background(), principal, "input-empty")
	if err != nil || len(input.Entries) != 0 {
		t.Fatalf("chat silently inherited applied context: input=%+v err=%v", input, err)
	}

	// Explicit composer attachments remain pinned even after the session-wide
	// applied-context gate is turned off.
	setID := "cs_" + opKey(principal, "apply-s4")
	disable := Command{OpID: "disable-s4", Action: "applyContext", SessionID: "s4", ExpectedVersion: applyReceipt.SessionVersion + 1, ExpectedContextSetID: setID, ExpectedBagVersion: 2, ExpectedSlotVersion: 1, BagID: "b4", Enabled: boolPtr(false), Selection: []EntryRef{}}
	disableReceipt := executeOK(t, svc, principal, disable)
	chatAttachment := Command{OpID: "chat-attached", Action: "pinInput", SessionID: "s4", ExpectedVersion: disableReceipt.SessionVersion, InputSnapshotID: "input-attached", RunID: "run-attached", Kind: "chat", DefinitionID: "assistant", DefinitionRevision: "r1", DefinitionDocument: testDefinition("chat"), Scope: &scope, Attachments: []EntryRef{{EntryID: "e4", RevisionID: "r4"}}}
	executeOK(t, svc, principal, chatAttachment)
	input, _, err = svc.ReadInput(context.Background(), principal, "input-attached")
	if err != nil || len(input.Entries) != 1 || input.Entries[0] != (EntryRef{EntryID: "e4", RevisionID: "r4"}) {
		t.Fatalf("explicit composer attachment was gated by applied context: input=%+v err=%v", input, err)
	}
}

func TestClearingInactiveBagPreservesAppliedSlotAndContextSet(t *testing.T) {
	svc, principal := newTestService(t, nil)
	created := createTestSession(t, svc, principal, "s5", "bag-a")
	scope := Scope{EnvironmentID: "uat", IdentityDomain: "tenant-a"}
	createB := Command{OpID: "create-bag-b", Action: "createBag", SessionID: "s5", ExpectedVersion: created.SessionVersion, BagID: "bag-b", Scope: &scope, Title: "Other bag"}
	createdB := executeOK(t, svc, principal, createB)
	initialContextSetID := "cs_" + opKey(principal, "create-s5")
	applyA := Command{OpID: "apply-a", Action: "applyContext", SessionID: "s5", ExpectedVersion: createdB.SessionVersion, ExpectedContextSetID: initialContextSetID, ExpectedBagVersion: 1, ExpectedSlotVersion: 0, BagID: "bag-a", Scope: &scope, Enabled: boolPtr(true), Selection: []EntryRef{}}
	applyReceipt := executeOK(t, svc, principal, applyA)
	activeSetID := "cs_" + opKey(principal, "apply-a")
	clearB := Command{OpID: "clear-b", Action: "clearBag", SessionID: "s5", ExpectedVersion: applyReceipt.SessionVersion, BagID: "bag-b", ExpectedBagVersion: 1, ExpectedSlotVersion: 1, Scope: &scope}
	clearReceipt := executeOK(t, svc, principal, clearB)
	snapshot, err := svc.GetSnapshot(context.Background(), principal, "s5")
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.Session.ContextSetID != activeSetID || snapshot.ContextSet.ID != activeSetID || len(snapshot.ContextSet.Slots) != 1 {
		t.Fatalf("clearing inactive bag changed the active context set: session=%+v set=%+v", snapshot.Session, snapshot.ContextSet)
	}
	active := snapshot.ContextSet.Slots[0]
	if active.BagID != "bag-a" || active.Version != 1 {
		t.Fatalf("clearing inactive bag changed the applied bag slot or its version: %+v", active)
	}
	var clearedBag Bag
	if err := svc.repo.View(context.Background(), func(r Reader) error {
		_, err := r.Load(context.Background(), Bags, "bag-b", &clearedBag)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if len(clearedBag.EntryIDs) != 0 || clearedBag.ClearVersion != clearedBag.Version || clearReceipt.SessionVersion != snapshot.Session.Version {
		t.Fatalf("inactive bag itself was not cleared or session receipt mismatch: bag=%+v receipt=%+v session=%+v", clearedBag, clearReceipt, snapshot.Session)
	}
}

func TestListSessionsFiltersAcrossOwnerPagesWithoutSkipping(t *testing.T) {
	svc, principal := newTestService(t, nil)
	other := principal
	other.ProjectID = "project2"
	for i := 0; i < 210; i++ {
		p := principal
		if i%2 == 0 {
			p = other
		}
		id := fmt.Sprintf("session-%03d", i)
		bag := fmt.Sprintf("bag-%03d", i)
		createTestSession(t, svc, p, id, bag)
	}
	first, err := svc.ListSessions(context.Background(), principal, "", 100)
	if err != nil || len(first.Items) != 100 || !first.HasMore || first.NextCursor != "session-200" {
		t.Fatalf("first scoped session page skipped or leaked records: len=%d cursor=%q more=%v err=%v", len(first.Items), first.NextCursor, first.HasMore, err)
	}
	second, err := svc.ListSessions(context.Background(), principal, first.NextCursor, 100)
	if err != nil || len(second.Items) != 5 || second.HasMore {
		t.Fatalf("second scoped session page is incomplete: len=%d cursor=%q more=%v err=%v", len(second.Items), second.NextCursor, second.HasMore, err)
	}
	for _, item := range append(first.Items, second.Items...) {
		if item.ProjectID != principal.ProjectID {
			t.Fatalf("session from another project leaked: %+v", item)
		}
	}
}

func TestListPagesBoundScanWorkAndSerializedBytes(t *testing.T) {
	svc, principal := newTestService(t, nil)
	ctx := context.Background()
	if err := svc.repo.Transact(ctx, func(tx Transaction) error {
		for i := 0; i < 1010; i++ {
			project := "project2"
			if i >= 1000 {
				project = principal.ProjectID
			}
			id := fmt.Sprintf("scan-%04d", i)
			item := Session{SchemaVersion: 1, ID: id, Owner: principal.Actor, DatabaseID: principal.DatabaseID, ProjectID: project, State: "active", Version: 1}
			if err := tx.Save(ctx, Sessions, id, item, 0, principal.Actor, id); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	first, err := svc.ListSessions(ctx, principal, "", 100)
	if err != nil || len(first.Items) != 0 || !first.HasMore || first.NextCursor != "scan-0999" {
		t.Fatalf("scan cap should return an empty progressing page: %+v err=%v", first, err)
	}
	second, err := svc.ListSessions(ctx, principal, first.NextCursor, 100)
	if err != nil || len(second.Items) != 10 || second.HasMore {
		t.Fatalf("filtered page after scan cap is incomplete: %+v err=%v", second, err)
	}

	created := createTestSession(t, svc, principal, "byte-page", "byte-bag")
	scope := Scope{EnvironmentID: "uat", IdentityDomain: "tenant-a"}
	pin := Command{OpID: "byte-pin", Action: "pinInput", SessionID: "byte-page", ExpectedVersion: created.SessionVersion, InputSnapshotID: "byte-input", RunID: "byte-run", Kind: "chat", DefinitionID: "assistant", DefinitionRevision: "r1", DefinitionDocument: testDefinition("chat"), Scope: &scope}
	executeOK(t, svc, principal, pin)
	if err := svc.repo.Transact(ctx, func(tx Transaction) error {
		for i := 0; i < 50; i++ {
			id := fmt.Sprintf("message-%03d", i)
			msg := Message{ID: id, SessionID: "byte-page", InputSnapshotID: "byte-input", Role: "assistant", Text: strings.Repeat("x", 90000)}
			if err := tx.Save(ctx, Messages, id, msg, 0, principal.Actor, "byte-page"); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	var total int
	cursor := ""
	for {
		page, err := svc.ListMessages(ctx, principal, "byte-page", cursor, 100)
		if err != nil {
			t.Fatal(err)
		}
		var pageBytes int
		for _, message := range page.Items {
			encoded, _ := json.Marshal(message)
			pageBytes += len(encoded)
		}
		if pageBytes > MaxSnapshotBytes {
			t.Fatalf("message page exceeded serialized byte bound: %d", pageBytes)
		}
		total += len(page.Items)
		if !page.HasMore {
			break
		}
		if page.NextCursor == "" || page.NextCursor == cursor {
			t.Fatalf("bounded message page did not advance: %+v", page)
		}
		cursor = page.NextCursor
	}
	if total != 51 {
		t.Fatalf("bounded message pages skipped or duplicated records: total=%d", total)
	}
}

func TestReplayReauthorizesReceiptScopeWhenCommandOmitsScope(t *testing.T) {
	denyScope := false
	svc, principal := newTestService(t, func(_ context.Context, access Access) error {
		if denyScope && access.Scope != nil && access.Scope.EnvironmentID == "uat" {
			return ErrForbidden
		}
		return nil
	})
	created := createTestSession(t, svc, principal, "s3", "b3")
	scope := Scope{EnvironmentID: "uat", IdentityDomain: "tenant-a"}
	command := Command{OpID: "entry-replay", Action: "putEntry", SessionID: "s3", ExpectedVersion: created.SessionVersion, BagID: "b3", ExpectedBagVersion: 1, EntryID: "e3", RevisionID: "r3", Subject: &Subject{ConceptID: "Order", FieldID: "Order.ID", FieldMappingRef: "mapping"}, Value: &ValueEnvelope{SchemaVersion: 1, Kind: "scalar", Scalar: valuePtr(apicontract.NewIntegerValue("3"))}}
	first := executeOK(t, svc, principal, command)
	replay := executeOK(t, svc, principal, command)
	if first.PayloadDigest != replay.PayloadDigest || first.ResourceVersion != replay.ResourceVersion {
		t.Fatalf("idempotent replay changed its receipt: first=%+v replay=%+v", first, replay)
	}
	if _, err := svc.Execute(context.Background(), principal, Command{OpID: command.OpID, Action: command.Action, SessionID: command.SessionID, ExpectedVersion: command.ExpectedVersion, BagID: command.BagID, ExpectedBagVersion: command.ExpectedBagVersion, EntryID: command.EntryID, RevisionID: command.RevisionID, Subject: command.Subject, Value: command.Value, Scope: &scope}); !errors.Is(err, ErrConflict) {
		t.Fatalf("same operation with changed payload must conflict, got %v", err)
	}
	denyScope = true
	if _, err := svc.Execute(context.Background(), principal, command); !errors.Is(err, ErrForbidden) {
		t.Fatalf("replay must reauthorize the stored receipt scope when request omits it, got %v", err)
	}
}

func TestPutEntryRejectsSnapshotOverBudgetWithoutMutation(t *testing.T) {
	svc, principal := newTestService(t, nil)
	created := createTestSession(t, svc, principal, "bounded-session", "bounded-bag")
	largeMapping := strings.Repeat("m", 700000)
	for i := 0; i < 3; i++ {
		value := apicontract.NewIntegerValue(fmt.Sprint(i))
		command := Command{
			OpID: fmt.Sprintf("bounded-put-%d", i), Action: "putEntry", SessionID: "bounded-session",
			ExpectedVersion: created.SessionVersion + int64(i), BagID: "bounded-bag", ExpectedBagVersion: int64(i + 1),
			EntryID: fmt.Sprintf("bounded-entry-%d", i), RevisionID: fmt.Sprintf("bounded-revision-%d", i),
			Subject: &Subject{ConceptID: "Order", FieldID: "Order.ID", FieldMappingRef: largeMapping},
			Value:   &ValueEnvelope{SchemaVersion: 1, Kind: "scalar", Scalar: &value},
		}
		_, err := svc.Execute(context.Background(), principal, command)
		if i < 2 && err != nil {
			t.Fatalf("entry %d unexpectedly exceeded the snapshot budget: %v", i, err)
		}
		if i == 2 && !errors.Is(err, ErrInvalid) {
			t.Fatalf("entry beyond the supported snapshot budget must be rejected, got %v", err)
		}
	}
	snapshot, err := svc.GetSnapshot(context.Background(), principal, "bounded-session")
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshot.Entries) != 2 || len(snapshot.Revisions) != 2 || snapshot.Bags[0].Version != 3 || snapshot.Session.Version != 3 {
		t.Fatalf("over-budget command partially mutated the session: %+v", snapshot)
	}
}

func TestRenameSessionAndBagUseVersionsAndKeepTitlesOutOfAudit(t *testing.T) {
	svc, principal := newTestService(t, nil)
	created := createTestSession(t, svc, principal, "rename-session", "rename-bag")
	renameSession := Command{OpID: "rename-session-title", Action: "renameSession", SessionID: "rename-session", ExpectedVersion: created.SessionVersion, Title: "Renamed session"}
	sessionReceipt := executeOK(t, svc, principal, renameSession)
	if sessionReceipt.ResourceID != "rename-session" || sessionReceipt.ResourceVersion != 2 {
		t.Fatalf("session rename receipt lost stable identity/version: %+v", sessionReceipt)
	}
	assertPrivateFieldsAbsentFromOperationMetadata(t, svc, principal, renameSession.OpID, "Renamed session")
	renameBag := Command{OpID: "rename-bag-title", Action: "renameBag", SessionID: "rename-session", ExpectedVersion: sessionReceipt.SessionVersion, BagID: "rename-bag", ExpectedBagVersion: 1, Title: "Orders"}
	bagReceipt := executeOK(t, svc, principal, renameBag)
	if bagReceipt.ResourceID != "rename-bag" || bagReceipt.ResourceVersion != 2 {
		t.Fatalf("bag rename receipt lost stable identity/version: %+v", bagReceipt)
	}
	assertPrivateFieldsAbsentFromOperationMetadata(t, svc, principal, renameBag.OpID, "Orders")
	stale := renameBag
	stale.OpID = "rename-bag-stale"
	if _, err := svc.Execute(context.Background(), principal, stale); !errors.Is(err, ErrConflict) {
		t.Fatalf("stale rename replay should conflict after changed payload, got %v", err)
	}
	invalid := renameSession
	invalid.OpID, invalid.ExpectedVersion, invalid.Title = "rename-blank", bagReceipt.SessionVersion, "  \t"
	if _, err := svc.Execute(context.Background(), principal, invalid); !errors.Is(err, ErrInvalid) {
		t.Fatalf("blank title accepted: %v", err)
	}
	invalid.OpID, invalid.Title = "rename-long", strings.Repeat("x", 257)
	if _, err := svc.Execute(context.Background(), principal, invalid); !errors.Is(err, ErrInvalid) {
		t.Fatalf("overlong title accepted: %v", err)
	}
	snapshot, err := svc.GetSnapshot(context.Background(), principal, "rename-session")
	if err != nil || snapshot.Session.Title != "Renamed session" || snapshot.Bags[0].Title != "Orders" {
		t.Fatalf("rename did not update the bounded snapshot: %+v err=%v", snapshot, err)
	}
}

func assertPrivateFieldsAbsentFromOperationMetadata(t *testing.T, svc *Service, p Principal, opID string, private ...string) {
	t.Helper()
	repo, ok := svc.repo.(*DALRepository)
	if !ok {
		t.Fatal("test service is not backed by DALRepository")
	}
	for _, collection := range []string{Operations, Audit} {
		data := map[string]any{}
		rec := record.NewRecordWithData(record.NewKeyWithID(collection, opKey(p, opID)), &data)
		if err := repo.db.Get(context.Background(), rec); err != nil {
			t.Fatalf("load %s record: %v", collection, err)
		}
		// Opaque digests can coincidentally contain short private strings; inspect
		// the receipt metadata while treating only its digest as non-reversible.
		if payloadJSON, ok := data["payload"].(string); ok {
			var payload map[string]any
			if err := json.Unmarshal([]byte(payloadJSON), &payload); err != nil {
				t.Fatalf("decode %s receipt: %v", collection, err)
			}
			delete(payload, "payloadDigest")
			data["payload"] = payload
		}
		delete(data, "recordId")
		encoded, err := json.Marshal(data)
		if err != nil {
			t.Fatal(err)
		}
		for _, value := range private {
			if strings.Contains(string(encoded), value) {
				t.Fatalf("%s metadata contains private value %q: %s", collection, value, encoded)
			}
		}
	}
}

func valuePtr(value apicontract.TypedValue) *apicontract.TypedValue { return &value }
func boolPtr(value bool) *bool                                      { return &value }

func testDefinition(kind string) json.RawMessage {
	if kind == "query" {
		return testQueryDefinition("select 1")
	}
	if kind == "dashboard" {
		return json.RawMessage(`{"schemaVersion":1,"kind":"dashboard","queries":[{"id":"q1","queryType":"SQL","text":"select 1"}],"widgets":[{"id":"w1","queryId":"q1"}]}`)
	}
	return json.RawMessage(fmt.Sprintf(`{"schemaVersion":1,"kind":%q,"queries":[]}`, kind))
}

func testQueryDefinition(text string) json.RawMessage {
	return json.RawMessage(fmt.Sprintf(`{"schemaVersion":1,"kind":"query","queries":[{"id":"q1","queryType":"SQL","text":%q}]}`, text))
}
