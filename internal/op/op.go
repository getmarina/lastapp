// Package op holds the transport-independent operation primitives. Console
// handlers and MCP tools call the same operations with an explicit Actor;
// neither transport fakes the other.
package op

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"

	mariadb "github.com/aantich/go-mariadb"

	"github.com/aantich/lastapp/internal/access"
	"github.com/aantich/lastapp/internal/entity"
	"github.com/aantich/lastapp/internal/registry"
)

// Actor is the caller of one operation: the live principal (authority is always
// rechecked against the registry), the request ID recorded in audit events and,
// for an MCP tool call, the delegated agent call that owns its receipt.
type Actor struct {
	Principal access.Principal
	RequestID string
	Agent     *access.AgentCall
}

// ErrActorAgentMismatch rejects an operation whose Actor does not carry the
// delegated call already in its context.
var ErrActorAgentMismatch = errors.New("operation actor does not carry the agent call of its context")

// WithAgentCall exposes a to the live authorization checks, which still read
// the delegated call from ctx until they take the Actor themselves. The Actor
// is the only source: ctx is never a fallback, and an Actor that disagrees
// with an agent call already in ctx fails rather than skipping its receipt.
func WithAgentCall(ctx context.Context, a *access.AgentCall) (context.Context, error) {
	switch current := access.AgentCallFrom(ctx); {
	case current == a:
		return ctx, nil
	case current != nil:
		return ctx, ErrActorAgentMismatch
	}
	return context.WithValue(ctx, access.AgentContextKey{}, a), nil
}

// ActorFrom is the shim for code not yet migrated to explicit actors: it reads
// the agent call that an MCP adapter placed in ctx.
func ActorFrom(ctx context.Context, p access.Principal, requestID string) Actor {
	return Actor{Principal: p, RequestID: requestID, Agent: access.AgentCallFrom(ctx)}
}

// AgentActor is the actor of one MCP tool call. Every call gets its own
// operation request ID, as an HTTP request would.
func AgentActor(ctx context.Context, p access.Principal) Actor {
	return ActorFrom(ctx, p, registry.NewID())
}

// Ref is the compact result of a mutation: the record and its new revision.
type Ref struct {
	ID       string `json:"id"`
	Revision int64  `json:"revision"`
}

// RevisionRequired rejects an update without an expected revision.
func RevisionRequired() error {
	return registry.Problem(428, "revision_required", "Send If-Match with the current resource revision.")
}

// opError is the one transport-independent classification of an operation
// error. HTTP writes payload() under "error"; MCP returns it in its tool result.
type Error struct {
	Status             int
	Code, Message      string
	Field, FieldOrRule string // empty when not applicable
	Rule               *string
	Retryable          bool
	// Authority carries the missing delegated scopes of needs_authorization.
	Authority *access.AgentAuthorityError
	// Unmapped marks an unexpected failure: log httpapi.Diagnostic server-side and
	// never return its details.
	Unmapped bool
}

// Transient reports a MariaDB deadlock (1213), lock wait timeout (1205) or
// snapshot conflict (1020: a repeatable-read transaction locked or changed a
// row another transaction committed after its snapshot, which
// innodb_snapshot_isolation rejects): the same work can simply be retried in a
// new transaction.
func Transient(err error) bool {
	var me *mariadb.MySQLError
	return errors.As(err, &me) && (me.Number == 1020 || me.Number == 1205 || me.Number == 1213)
}

func Classify(err error) Error {
	var ae *registry.APIError
	var ed entity.Diagnostic
	var me *mariadb.MySQLError
	if errors.As(err, &ae) {
	} else if errors.As(err, &ed) {
		ae = &registry.APIError{Status: 400, Code: ed.Code, Message: ed.Message}
	} else if Transient(err) {
		ae = &registry.APIError{Status: 409, Code: "retry_transaction", Message: "A concurrent change occurred; refresh and retry."}
	} else if errors.As(err, &me) {
		switch me.Number {
		case 1062:
			ae = &registry.APIError{Status: 409, Code: "already_exists", Message: "This ID or name already exists. Read the existing resource before retrying."}
		case 1451, 1452:
			ae = &registry.APIError{Status: 409, Code: "invalid_reference", Message: "A referenced resource is missing, belongs to another Tenant, or is still in use."}
		case 4025:
			ae = &registry.APIError{Status: 400, Code: "invalid_request", Message: "The request violates a resource constraint."}
		}
	}
	o := Error{}
	if ae == nil {
		ae = &registry.APIError{Status: 500, Code: "internal_error", Message: "The operation failed. See the server log with this request ID."}
		o.Unmapped = true
	}
	o.Status, o.Code, o.Message = ae.Status, ae.Code, ae.Message
	o.Retryable = ae.Code == "retry_transaction" || ae.Code == "publication_in_progress" || ae.Status == 429
	if ed.Path != "" {
		o.FieldOrRule = ed.Path
		if ed.Code == "invalid_value" {
			o.Field = ed.Path
		}
	}
	var rf entity.RuleFailure
	if errors.As(err, &rf) {
		if rf.Field != "" {
			o.Field = rf.Field
		}
		o.Rule = &rf.Rule
	}
	if ae.Code == "needs_authorization" {
		var authority *access.AgentAuthorityError
		if errors.As(err, &authority) {
			o.Authority = authority
		}
	}
	return o
}

func (o Error) Payload(requestID string) registry.Row {
	v := registry.Row{"code": o.Code, "message": o.Message, "request_id": requestID, "retryable": o.Retryable}
	if o.FieldOrRule != "" {
		v["field_or_rule"] = o.FieldOrRule
	}
	if o.Field != "" {
		v["field"] = o.Field
	}
	if o.Rule != nil {
		v["rule"] = *o.Rule
	}
	return v
}

// Runner is the mutation core shared by every operation: one READ COMMITTED
// transaction that locks the Tenant (or the platform SuperRole) row first,
// rechecks live authorization and the production guard, then writes the agent
// receipt together with the change and its audit events.
type Runner struct {
	DB *sql.DB
	// Access provides the live authorization checks (authorize, scopes, guards).
	Access *access.Checker
}

func (r *Runner) Mutate(ctx context.Context, a Actor, action string, res access.Resource, fn func(*sql.Tx) error) error {
	ctx, e := WithAgentCall(ctx, a.Agent)
	if e != nil {
		return e
	}
	tx, e := r.DB.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	if e != nil {
		return e
	}
	defer tx.Rollback()
	if res.Tenant != "" {
		if e = registry.LockTenant(ctx, tx, res.Tenant); e != nil {
			return registry.Missing()
		}
	} else {
		var x string
		if e = tx.QueryRowContext(ctx, "SELECT id FROM roles WHERE id=? FOR UPDATE", access.SuperRole).Scan(&x); e != nil {
			return e
		}
	}
	if e = r.Access.Authorize(ctx, tx, a.Principal, action, res, true); e != nil {
		return e
	}
	if e = r.Access.ProductionMutationGuard(ctx, tx, a.Principal, action, res); e != nil {
		return e
	}
	if e = agentMutationBegin(ctx, tx, a.Agent); e != nil {
		return e
	}
	if e = fn(tx); e != nil {
		return e
	}
	if e = agentMutationFinish(ctx, tx, a.Agent); e != nil {
		return e
	}
	if e = tx.Commit(); e != nil {
		return e
	}
	if a.Agent != nil {
		a.Agent.Committed = true
	}
	return nil
}

// Authorize is a read-side live authorization check for a.
func (r *Runner) Authorize(ctx context.Context, a Actor, action string, res access.Resource) error {
	return r.Check(ctx, r.DB, a, action, res, false)
}

// AuthorizeTx rechecks a's mutating authority inside an operation transaction.
func (r *Runner) AuthorizeTx(ctx context.Context, tx *sql.Tx, a Actor, action string, res access.Resource) error {
	return r.Check(ctx, tx, a, action, res, true)
}

// Check is the live authorization check on q. mutating selects the stricter
// membership and platform-control rules of a change, also before its transaction.
func (r *Runner) Check(ctx context.Context, q registry.Queryer, a Actor, action string, res access.Resource, mutating bool) error {
	ctx, e := WithAgentCall(ctx, a.Agent)
	if e != nil {
		return e
	}
	return r.Access.Authorize(ctx, q, a.Principal, action, res, mutating)
}

// OperationID is the identifier of the record a's operation creates: the
// receipt's reserved result ID for an agent call, a new ID otherwise.
func (a Actor) OperationID() string {
	if a.Agent != nil {
		return registry.Str(a.Agent.ResultRef["id"])
	}
	return registry.NewID()
}

// Scopes returns a's live read scopes in tenant; apply them before paging.
func (r *Runner) Scopes(ctx context.Context, a Actor, tenant, action string) (access.ScopeSet, error) {
	ctx, e := WithAgentCall(ctx, a.Agent)
	if e != nil {
		return access.ScopeSet{}, e
	}
	return r.Access.Scopes(ctx, r.DB, a.Principal, tenant, action)
}

// Ref reads a record's compact result after its mutation committed.
func (r *Runner) Ref(ctx context.Context, table, tenant, record string) (Ref, error) {
	query := "SELECT id,revision FROM " + table + " WHERE id=?"
	args := []any{record}
	if tenant != "" {
		query += " AND tenant_id=?"
		args = append(args, tenant)
	}
	var v Ref
	if e := r.DB.QueryRowContext(ctx, query, args...).Scan(&v.ID, &v.Revision); e != nil {
		if errors.Is(e, sql.ErrNoRows) {
			return v, registry.Missing()
		}
		return v, e
	}
	return v, nil
}

// AuditAs records an audit event attributed to a: the human user, the request
// ID and, for delegated calls, the agent grant and client. Workers carry the
// grant identity of their persisted job through agentAuditContext.
func AuditAs(ctx context.Context, q registry.Queryer, a Actor, tenant, action, kind, record string, details any) error {
	identity, _ := ctx.Value(agentAuditKey{}).(agentAuditIdentity)
	if a.Agent != nil {
		identity = agentAuditIdentity{Grant: a.Agent.Grant.ID, Client: a.Agent.Grant.Client}
	}
	if identity.Grant != "" {
		details = registry.Row{"agent_grant_id": identity.Grant, "agent_client_id": identity.Client, "details": details}
	}
	b, _ := json.Marshal(details)
	_, e := q.ExecContext(ctx, "INSERT INTO audit_events (tenant_id,actor_user_id,action,resource_type,resource_id,request_id,details_json) VALUES (?,?,?,?,?,?,?)", registry.Nullable(tenant), registry.Nullable(a.Principal.ID), action, kind, registry.Nullable(record), a.RequestID, string(b))
	return e
}

func CheckRevision(got, want int64) error {
	if got != want {
		return registry.Problem(412, "stale_revision", "This resource changed. Refresh before saving.")
	}
	return nil
}

// Audit is the context shim of auditAs for code not yet migrated to actors.
func Audit(ctx context.Context, q registry.Queryer, tenant, user, action, kind, record, request string, details any) error {
	return AuditAs(ctx, q, Actor{Principal: access.Principal{ID: user}, RequestID: request, Agent: access.AgentCallFrom(ctx)}, tenant, action, kind, record, details)
}

const agentMaxReceipts = 10000

// Audit identity is metadata, never an authorization credential. Workers carry
// it from persisted jobs, including after revocation or recovery.
type agentAuditKey struct{}

type agentAuditIdentity struct{ Grant, Client string }

func AgentAuditContext(ctx context.Context, grant, client string) context.Context {
	if grant == "" {
		return ctx
	}
	return context.WithValue(ctx, agentAuditKey{}, agentAuditIdentity{Grant: grant, Client: client})
}

var ErrAgentReplay = errors.New("agent operation already committed")

func agentMutationBegin(ctx context.Context, tx *sql.Tx, a *access.AgentCall) error {
	if a == nil {
		return nil
	}
	if a.Key == "" {
		return registry.Bad("request_key is required for every mutation")
	}
	var previous string
	var saved []byte
	e := tx.QueryRowContext(ctx, "SELECT input_hash,result_json FROM agent_operation_receipts WHERE grant_id=? AND request_key=? FOR UPDATE", a.Grant.ID, a.Key).Scan(&previous, &saved)
	if e == nil {
		if previous != a.Hash {
			return registry.Conflict("request_key was already used for different arguments")
		}
		var result registry.Row
		if e = json.Unmarshal(saved, &result); e != nil {
			return e
		}
		a.ResultRef = result
		return ErrAgentReplay
	}
	if !errors.Is(e, sql.ErrNoRows) {
		return e
	}
	var n int
	if e = tx.QueryRowContext(ctx, "SELECT COUNT(*) FROM agent_operation_receipts WHERE grant_id=?", a.Grant.ID).Scan(&n); e != nil {
		return e
	}
	if n >= agentMaxReceipts {
		return registry.Problem(403, "agent_limit", "This connection has reached its operation limit. Approve a new connection.")
	}
	return nil
}

func agentMutationFinish(ctx context.Context, tx *sql.Tx, a *access.AgentCall) error {
	if a == nil {
		return nil
	}
	b, e := json.Marshal(a.ResultRef)
	if e != nil {
		return e
	}
	_, e = tx.ExecContext(ctx, "INSERT INTO agent_operation_receipts(grant_id,request_key,input_hash,operation,result_json) VALUES(?,?,?,?,?)", a.Grant.ID, a.Key, a.Hash, a.Operation, b)
	if e == nil {
		_, e = tx.ExecContext(ctx, "UPDATE agent_grants SET last_used_at=UTC_TIMESTAMP(6) WHERE id=?", a.Grant.ID)
	}
	return e
}

type Done struct {
	OK bool `json:"ok"`
}

type NamedInput struct {
	ID   string `json:"id"`
	Slug string `json:"slug"`
	Name string `json:"display_name"`
}
