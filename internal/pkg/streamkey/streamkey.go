package streamkey

// Redis stream key constants shared by publishers and consumers.
const (
	OGFetch  = "stream:og-fetch"
	ClickLog = "stream:click-log"
	ClickDLQ = "stream:click-dlq"
)
