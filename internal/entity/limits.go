package entity

// Semantic limits have one source, shared with the editor capability contract
// and the SQL adapter. In particular, literal casts must preserve every allowed
// decimal value exactly, including the largest integer part.
const (
	MaxTenantDataRequests           = 16
	MaxDecimalPrecision             = 30
	MaxDecimalScale                 = 12
	MaxInteger                      = 9007199254740991
	MinTextLength                   = 1
	MaxTextLength                   = 1000
	MaxIndexedTextLength            = 512
	MaxChoiceValues                 = 100
	MaxJSONBytes                    = 16384
	MaxJSONDepth                    = 16
	MaxJSONNodes                    = 4096
	MaxSQLParameters                = 8192
	MaxPublicationValidationRecords = 10000
	MaxActorTeams                   = 1000
)

func FieldConstraints() any {
	return map[string]any{"max_decimal_precision": MaxDecimalPrecision, "max_decimal_scale": MaxDecimalScale, "max_integer": int64(MaxInteger), "min_text_length": MinTextLength, "max_text_length": MaxTextLength, "max_indexed_text_length": MaxIndexedTextLength, "max_choice_values": MaxChoiceValues, "max_json_bytes": MaxJSONBytes, "max_json_depth": MaxJSONDepth, "max_json_nodes": MaxJSONNodes, "max_query_parameters": MaxSQLParameters, "max_publication_validation_records": MaxPublicationValidationRecords, "max_actor_teams": MaxActorTeams, "max_tenant_data_requests": MaxTenantDataRequests, "max_read_response_bytes": MaxReadResponseBytes, "max_definition_bytes": MaxDefinitionBytes, "max_description_bytes": MaxDescriptionBytes}
}

const (
	MaxReadResponseBytes = 2 << 20
	MaxDefinitionBytes   = 1 << 20
	MaxDescriptionBytes  = 4000
)

// Conservative encoded width, including JSON escaping and field/map overhead.
func EstimatedRecordBytes(fields []Field) int {
	size := 512
	for _, field := range fields {
		width := 1024
		switch field.Type {
		case "text":
			width = field.MaxLength*6 + 2
		case "json":
			width = MaxJSONBytes
		case "integer", "decimal":
			width = 64
		case "boolean":
			width = 5
		case "user", "team", "reference":
			width = 40
		case "date", "timestamp":
			width = 40
		}
		size += width + len(field.Key) + 128
	}
	return size
}

const MaxSpaceEntities = 100
