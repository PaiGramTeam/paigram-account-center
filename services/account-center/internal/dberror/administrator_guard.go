package dberror

const activeAdministratorConstraint = "active_administrator_required"

// IsActiveAdministratorGuardViolation reports whether a mutation would remove the last recovery-capable administrator.
func IsActiveAdministratorGuardViolation(err error) bool {
	return IsCheckConstraint(err, activeAdministratorConstraint)
}
