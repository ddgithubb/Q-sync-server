package sstypes

func BuildSSMessage(op SSMessage_Op, data SSMessageData) *SSMessage {
	return &SSMessage{
		Op:   op,
		Data: data,
	}
}

func BuildSSMessageWithKey(op SSMessage_Op, key string, data SSMessageData) *SSMessage {
	return &SSMessage{
		Op:   op,
		Key:  key,
		Data: data,
	}
}