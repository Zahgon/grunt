package yaml

// yamlType is js-yaml's Type (lib/js-yaml/type.js), reduced to what loading needs.
// resolve receives the node's data, which for a scalar is normally a string or nil;
// when a tag on its own line is followed by a plain scalar, js-yaml resolves the
// scalar implicitly first and hands the explicit type the resulting value, so
// resolvers also accept numbers, booleans and dates, as their JavaScript originals do.
type yamlType struct {
	tag       string
	kind      string
	resolve   func(data any) bool
	construct func(data any) (any, error)
}

// schema is a compiled js-yaml Schema: implicit types in resolution order and the
// type map by kind, with "fallback" holding every type.
type schema struct {
	implicit []*yamlType
	typeMap  map[string]map[string]*yamlType
}

// newSchema compiles a schema from its implicit and explicit types, in the order
// js-yaml's compileList produces for the default schemas (included schemas first).
func newSchema(implicit, explicit []*yamlType) *schema {
	s := &schema{
		implicit: implicit,
		typeMap: map[string]map[string]*yamlType{
			kindScalar: {}, kindSequence: {}, kindMapping: {}, "fallback": {},
		},
	}
	for _, list := range [][]*yamlType{implicit, explicit} {
		for _, t := range list {
			s.typeMap[t.kind][t.tag] = t
			s.typeMap["fallback"][t.tag] = t
		}
	}
	return s
}

// tagMerge is the tag of "<<" merge keys.
const tagMerge = "tag:yaml.org,2002:merge"

var (
	// failsafe + json + core + default_safe implicit types, in compiled order.
	safeImplicit = []*yamlType{nullType, boolType, intType, floatType, timestampType, mergeType}
	safeExplicit = []*yamlType{strType, seqType, mapType, binaryType, omapType, pairsType, setType}

	// defaultSafeSchema is DEFAULT_SAFE_SCHEMA (lib/js-yaml/schema/default_safe.js).
	defaultSafeSchema = newSchema(safeImplicit, safeExplicit)

	// defaultFullSchema is DEFAULT_FULL_SCHEMA (lib/js-yaml/schema/default_full.js).
	defaultFullSchema = newSchema(safeImplicit,
		append(append([]*yamlType(nil), safeExplicit...), undefinedType, regexpType, functionType))
)
