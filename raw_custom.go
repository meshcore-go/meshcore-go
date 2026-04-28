package meshcore

type RawCustom struct {
	Data []byte
}

func RawCustomFromBytes(data []byte) (*RawCustom, error) {
	copied := make([]byte, len(data))
	copy(copied, data)
	return &RawCustom{
		Data: copied,
	}, nil
}

func (r *RawCustom) ToBytes() ([]byte, error) {
	copied := make([]byte, len(r.Data))
	copy(copied, r.Data)
	return copied, nil
}
