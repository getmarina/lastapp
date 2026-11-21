package op

import (
	"context"
	"strings"

	"github.com/aantich/lastapp/internal/registry"
)

type PageInfo struct {
	Page    int  `json:"page"`
	Limit   int  `json:"limit"`
	HasMore bool `json:"has_more"`
}

// PageRequest is a bounded collection request. Zero Page/Limit select the
// defaults; any other value outside the bounds is rejected by info().
type PageRequest struct {
	Page, Limit int
	Search      string
}

// Page is one bounded page of a collection.
type Page[T any] struct {
	Items      []T      `json:"items"`
	Pagination PageInfo `json:"pagination"`
}

// Every collection is bounded, including default reads. Permission predicates
// stay inside the original query, before search and LIMIT/OFFSET.
func (r PageRequest) info() (PageInfo, string, error) {
	info := PageInfo{Page: 1, Limit: 100}
	if r.Page != 0 {
		if r.Page < 1 || r.Page > 10000 {
			return info, "", registry.Bad("Page must be between 1 and 10000.")
		}
		info.Page = r.Page
	}
	if r.Limit != 0 {
		if r.Limit < 1 || r.Limit > 200 {
			return info, "", registry.Bad("Limit must be between 1 and 200.")
		}
		info.Limit = r.Limit
	}
	if len(r.Search) > 120 {
		return info, "", registry.Bad("Search must be at most 120 bytes.")
	}
	return info, r.Search, nil
}

// Collection names a paged collection's searchable output columns and its
// stable order. Only these static names enter SQL; search text is bound.
type Collection struct{ columns, order string }

var defaultCollection = Collection{"id,display_name", "display_name,id"}

// collections maps each collection key to its search columns and order.
var collections = map[string]Collection{
	"me":                  {"id,display_name,slug,state", "display_name,id"},
	"package-catalog":     {"package_key,version,display_name", "package_key,version"},
	"package-stages":      {"package_key,version,stage_state", "workspace_id"},
	"my-apps":             {"display_name", "display_name,id"},
	"my-tenants":          {"id,display_name,slug,state", "display_name,id"},
	"tenants":             {"id,display_name,slug,state", "display_name,id"},
	"plans":               defaultCollection,
	"database-resources":  {"id,database_name,state", "created_at DESC,id"},
	"my-role-assignments": {"id,display_name,code,scope_kind,scope_team_id,space_id,app_id", "scope_kind,id"},
	"role-assignments":    {"id,principal_name,access_name,scope_name", "created_at DESC,id"},
	// An App's own assignments ('app') before its Space's ('space').
	"app-access-assignments": {"id,principal_name,access_name,scope_name", "scope_kind,created_at DESC,id"},
	"memberships":            {"user_id,display_name,email,state", "display_name,user_id"},
	"administrators":         {"id,user_id,display_name,email,state", "display_name,id"},
	"recipients":             {"id,user_id,display_name,email", "display_name,id"},
	"team-recipients":        defaultCollection,
	"members":                {"user_id,display_name", "display_name,user_id"},
	"audit-events":           {"id,action,resource_type,resource_id,actor_name,details_json", "id DESC"},
	"roles":                  {"id,display_name,code,state", "protected DESC,display_name,id"},
	"builds":                 {"id,content_hash,entrypoint", "created_at DESC,id"},
	"releases":               {"id,display_name", "created_at DESC,id"},
	"workspaces":             {"id,display_name", "created_at DESC,id"},
	"spaces":                 {"id,display_name,slug,state", "created_at DESC,id"},
	"apps":                   {"id,display_name,slug,state", "created_at DESC,id"},
	"microapps":              {"id,display_name,slug,state", "navigation_order,created_at DESC,id"},
	"teams":                  {"id,display_name,slug,state", "display_name,id"},
	"users":                  {"id,display_name,email,oidc_issuer,oidc_subject,state", "display_name,id"},
	"agent-access":           {"id,client_name,client_id,scope_kind,state", "created_at DESC,id"},
}

func CollectionFor(key string) Collection {
	if c, ok := collections[key]; ok {
		return c
	}
	return defaultCollection
}

// PageScan runs one bounded page of query, an already permission-filtered
// statement, searching and ordering by the collection's static columns, and
// scans it into typed registry records.
func PageScan[T any, P registry.Record[T]](ctx context.Context, q registry.Queryer, req PageRequest, c Collection, query string, args ...any) (Page[T], error) {
	info, query, args, e := PageSQL(req, c, query, args)
	if e != nil {
		return Page[T]{}, e
	}
	v, e := registry.ScanAll[T, P](ctx, q, query, args...)
	if e != nil {
		return Page[T]{}, e
	}
	return PageOf(info, v), nil
}

// PageSQL wraps query in the collection's search, order and one-extra LIMIT.
func PageSQL(req PageRequest, c Collection, query string, args []any) (PageInfo, string, []any, error) {
	info, search, e := req.info()
	if e != nil {
		return info, "", nil, e
	}
	query = "SELECT * FROM (" + query + ") AS collection"
	if search != "" {
		search = strings.NewReplacer("=", "==", "%", "=%", "_", "=_").Replace(search)
		query += " WHERE CONCAT_WS(' '," + c.columns + ") LIKE ? ESCAPE '='"
		args = append(args, "%"+search+"%")
	}
	query += " ORDER BY " + c.order + " LIMIT ? OFFSET ?"
	args = append(args, info.Limit+1, (info.Page-1)*info.Limit)
	return info, query, args, nil
}

// PageOf cuts the one-extra row that reports has_more.
func PageOf[T any](info PageInfo, v []T) Page[T] {
	info.HasMore = len(v) > info.Limit
	if info.HasMore {
		v = v[:info.Limit]
	}
	return Page[T]{Items: v, Pagination: info}
}

// Searchable is a metadata item that an inventory search matches against.
type Searchable interface{ SearchText() string }

// Only pass already-authorized metadata here. Filtering must precede paging;
// serialize the resulting page rather than an oversized complete inventory.
func PageItems[T Searchable](items []T, req PageRequest) ([]T, PageInfo, error) {
	info, search, err := req.info()
	if err != nil {
		return nil, info, err
	}
	filtered := []T{}
	for _, v := range items {
		if search != "" {
			if !strings.Contains(v.SearchText(), strings.ToLower(search)) {
				continue
			}
		}
		filtered = append(filtered, v)
	}
	start := min((info.Page-1)*info.Limit, len(filtered))
	end := min(start+info.Limit, len(filtered))
	info.HasMore = end < len(filtered)
	return filtered[start:end], info, nil
}
