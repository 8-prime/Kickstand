package trip

import _ "embed"

// JSONSchema is the published contract for a trip document, served at
// /api/schema/trip.json. It is the thing to hand a model when you want it to
// write a new trip.
//
//go:embed schema.json
var JSONSchema []byte
