package events

// Subjects de NATS JetStream.
// Nomenclatura: posnet.{dominio}.{evento}.{versión}
//
// Todos los archivos del paquete (payloads, registry, envelope) referencian
// estas constantes. Centralizar aquí evita strings duplicados y typos.
const (
	SubjectTransactionReceived    = "posnet.transaction.received.v1"
	SubjectReversalRequested      = "posnet.transaction.reversal-requested.v1"
	SubjectBatchCloseRequested    = "posnet.transaction.batch-close.v1"
	SubjectFraudCheckRequested    = "posnet.fraud.check-requested.v1"
	SubjectFraudScoreCalculated   = "posnet.fraud.score-calculated.v1"
	SubjectAuthApproved           = "posnet.auth.approved.v1"
	SubjectAuthRejected           = "posnet.auth.rejected.v1"
	SubjectReversalCompleted      = "posnet.auth.reversal-completed.v1"
	SubjectBatchClosed            = "posnet.settlement.batch-closed.v1"
	SubjectSettlementCompleted    = "posnet.settlement.completed.v1"
	SubjectNotificationDispatched = "posnet.notification.dispatched.v1"
)
