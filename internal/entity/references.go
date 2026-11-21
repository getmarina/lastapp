package entity

// RewriteReferences visits the identity-bearing positions shared by workspace
// copies and portable packages. Expressions, labels, subjects, and literals
// retain their original meaning. Callers supply the lookup policy.
func (d *Definition) RewriteReferences(target, role func(string) string) {
	for i := range d.Fields {
		f := &d.Fields[i]
		if f.Type == "reference" {
			f.Target = target(f.Target)
		}
	}
	for i := range d.Permissions {
		for j, id := range d.Permissions[i].Roles {
			d.Permissions[i].Roles[j] = role(id)
		}
	}
}

// RewriteGateReferences visits App access permission and role references.
func (a *AppAccess) RewriteGateReferences(permission, role func(string) string) {
	for _, gates := range [][]Gate{a.Ceiling, a.Restrictions} {
		for i := range gates {
			gates[i].Permission = permission(gates[i].Permission)
			for j, id := range gates[i].Roles {
				gates[i].Roles[j] = role(id)
			}
		}
	}
}
