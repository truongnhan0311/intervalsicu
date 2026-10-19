package intervalsicu

import (
	"reflect"
	"strings"
	"sync"
)

// knownActivityFields lists the JSON names [Activity] decodes into real fields.
//
// Derived from the struct by reflection rather than written out by hand, so
// adding a field can never leave a stale copy behind that quietly routes the
// same key into both the field and Extra.
var knownActivityFields = sync.OnceValue(func() map[string]bool {
	return jsonFieldNames(reflect.TypeFor[activityAlias]())
})

// jsonFieldNames collects the JSON names of a struct's exported fields.
func jsonFieldNames(structType reflect.Type) map[string]bool {
	names := make(map[string]bool, structType.NumField())

	for i := range structType.NumField() {
		field := structType.Field(i)
		tag := field.Tag.Get("json")
		if tag == "-" {
			continue
		}

		name, _, _ := strings.Cut(tag, ",")
		if name == "" {
			name = field.Name
		}
		names[name] = true
	}

	return names
}
