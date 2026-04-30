package mail

type InboundS3Event struct {
	Records []InboundS3Record `json:"Records"`
}

type InboundS3Record struct {
	EventSource string         `json:"eventSource"`
	S3          InboundS3Value `json:"s3"`
}

type InboundS3Value struct {
	Bucket InboundS3Bucket `json:"bucket"`
	Object InboundS3Object `json:"object"`
}

type InboundS3Bucket struct {
	Name string `json:"name"`
}

type InboundS3Object struct {
	Key string `json:"key"`
}
