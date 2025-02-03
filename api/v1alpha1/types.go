package v1alpha1

// Definitions to manage status condition types
const (
	// ConditionReconciled represents the status of the resource reconciliation
	ConditionReconciled = "Reconciled"
)

// Definitions to manage status condition reasons
const (
	Reconciling     = "Reconciling"
	Unreachable     = "Unreachable"
	CreationFailure = "CreationFailure"
	Reconciled      = "Reconciled"
	DeletionFailure = "DeletionFailure"
	DeletionBlocked = "DeletionBlocked"
)
