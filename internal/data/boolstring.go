package data

type BoolString bool

func (b *BoolString) UnmarshalJSON(data []byte) error {
	if string(data) == "1" {
		*b = true
	} else {
		*b = false
	}
	return nil
}
