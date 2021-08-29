package parser

type FieldName string

// A Field represents an unprocessed field of a single event. The Name is the field's identifier, which is used to
// process the fields afterwards. The Value is not owned by the Field struct.
type Field struct {
	Name  FieldName
	Value []byte
}

// IsEventEnd returns true if the field is EventEnd.
func (f *Field) IsEventEnd() bool {
	return f.Name == EventEnd.Name
}

const (
	FieldNameData  = FieldName("data")
	FieldNameEvent = FieldName("event")
	FieldNameRetry = FieldName("retry")
	FieldNameID    = FieldName("id")

	maxFieldNameLength = 5
)

// EventEnd is not an actual field. If a parser's Field method returns an EventEnd
// field it means that all the fields parsed before this one are part of a single event.
//
// The EventEnd field has no meaning outside parsing.
var EventEnd = Field{}

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
