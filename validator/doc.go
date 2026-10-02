// Package validator validates input in two styles that share one error type:
//
//   - struct tags, through an [Engine] wrapping go-playground/validator, with
//     a "date" tag and optional JSON field names;
//   - small composable checks for single values ([EmailValidator],
//     [PhoneValidator], [UUIDValidator], [TimeValidator]) combined with
//     [ChainValidator] or [CollectAll].
//
// Every validation failure is reported as a [ValidationError] value that maps
// field names to messages, ready to be returned to an API client. Other
// errors (for example passing a non-struct to Engine.Struct) are returned
// unchanged so that callers can tell bad input from programming mistakes.
package validator
