package v1alpha1

// Definitions to manage status condition types
const (
	ConditionAvailable   = "Available"   // The resource is in sync with S3
	ConditionProgressing = "Progressing" // The resource is being synced with S3
	ConditionDegraded    = "Degraded"    // The sync with S3 has failed, resource state is unknown
	ConditionRejected    = "Rejected"    // The resource cannot be sync with S3 and does not exists on S3
)

// Definitions to manage status condition reasons
const (
	Reconciling     = "Reconciling"
	InternalError   = "OperatorError"
	K8sApiError     = "KubernetesAPIError"
	Unreachable     = "Unreachable"
	CreationFailure = "CreationFailure"
	Reconciled      = "Reconciled"
	DeletionFailure = "DeletionFailure"
	DeletionBlocked = "DeletionBlocked"
)
