package parser_test

import (
	"testing"

	"github.com/tmaxmax/go-sse/internal/parser"
)

func newField(tb testing.TB, name parser.FieldName, value string) parser.Field {
	tb.Helper()

	return parser.Field{
		Name:  name,
		Value: value,
	}
}

func newDataField(tb testing.TB, value string) parser.Field {
	tb.Helper()

	return newField(tb, parser.FieldNameData, value)
}

func newEventField(tb testing.TB, value string) parser.Field {
	tb.Helper()

	return newField(tb, parser.FieldNameEvent, value)
}

func newRetryField(tb testing.TB, value string) parser.Field {
	tb.Helper()

	return newField(tb, parser.FieldNameRetry, value)
}

func newIDField(tb testing.TB, value string) parser.Field {
	tb.Helper()

	return newField(tb, parser.FieldNameID, value)
}

func newCommentField(tb testing.TB, value string) parser.Field {
	tb.Helper()

	return newField(tb, parser.FieldNameComment, value)
}

const benchmarkText = `
event:cycles
data:8
id:10

event:ops
data:2
id:11

data:10667007354186551956
id:12

event:cycles
data:9
id:13

event:cycles
data:10
id:14

event:cycles
data:11
id:15

event:cycles
data:12
id:16

event:ops
data:3
id:17

event:cycles
data:13
id:18

data:4751997750760398084
id:19

event:cycles
data:14
id:20

event:cycles
data:15
id:21

event:cycles
data:16
id:22

event:ops
data:4
id:23

event:cycles
data:17
id:24

event:cycles
data:18
id:25

data:3510942875414458836
data:12156940908066221323
data:4324745483838182873
id:26

event:cycles
data:19
id:27

event:cycles
data:20
id:28

event:ops
data:5
id:29

event:cycles
data:21
id:30

event:cycles
data:22
id:31

event:cycles
data:23
id:32

data:6263450610539110790
id:33

event:ops
data:6
id:34

event:cycles
data:24
id:35

event:cycles
data:25
id:36

event:cycles
data:26
id:37

event:cycles
data:27
id:38

data:3328451335138149956
id:39

event:ops
data:7
id:40

event:cycles
data:28
id:41

event:cycles
data:29
id:42

event:cycles
data:30
id:43

event:cycles
data:31
id:44

`
