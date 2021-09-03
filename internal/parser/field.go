package parser

type FieldName string

// A Field represents an unprocessed field of a single event. The Name is the field's identifier, which is used to
// process the fields afterwards. The Value is not owned by the Field struct.
//
// As a special case, if a parser (ByteParser or ReaderParser) returns a field without a name,
// it means that a whole event was parsed. In other words, all the fields before the one without a name
// and after another such field are part of the same event.
type Field struct {
	Name  FieldName
	Value []byte
}

const (
	FieldNameData  = FieldName("data")
	FieldNameEvent = FieldName("event")
	FieldNameRetry = FieldName("retry")
	FieldNameID    = FieldName("id")

	maxFieldNameLength = 5
)

func getFieldName(b []byte) (FieldName, bool) {
	if len(b) == 0 {
		return "", false
	}
	switch FieldName(trimNewline(b)) {
	case FieldNameData:
		return FieldNameData, true
	case FieldNameEvent:
		return FieldNameEvent, true
	case FieldNameRetry:
		return FieldNameRetry, true
	case FieldNameID:
		return FieldNameID, true
	default:
		return "", b[0] == '\n' || b[0] == '\r'
	}
}
