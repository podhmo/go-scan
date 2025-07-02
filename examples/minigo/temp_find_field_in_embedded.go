package main

import (
	"fmt"
	"go/token"
)

// findFieldInEmbedded recursively searches for a field within the embedded structs of a given StructInstance.
// It returns the found Object, or nil if not found.
// It also returns an error for ambiguity or other issues.
func findFieldInEmbedded(instance *StructInstance, fieldName string, fset *token.FileSet, selPos token.Pos) (Object, error) {
	var foundObj Object
	var foundInDefName string // Name of the embedded struct type where the field was found

	for _, embDef := range instance.Definition.EmbeddedDefs {
		// Get the actual instance of this embedded struct type from the parent instance
		embInstance, embInstanceExists := instance.EmbeddedValues[embDef.Name]
		if !embInstanceExists {
			// This case implies an embedded struct that wasn't initialized (e.g. Outer{ Embed OnlyOuterField: 1})
			// If Outer embeds Inner, but Inner itself wasn't part of the literal for Outer,
			// then instance.EmbeddedValues[embDef.Name] might not exist.
			// For now, if the embedded instance doesn't exist, we can't find fields in it.
			// A more Go-like behavior might be that embInstance is implicitly a zero-value struct.
			// For now, skip if not found.
			continue
		}

		// 1. Check direct fields of the current embedded instance
		if val, ok := embInstance.FieldValues[fieldName]; ok {
			if foundObj != nil { // Ambiguity
				return nil, formatErrorWithContext(fset, selPos,
					fmt.Errorf("ambiguous selector %s (found in both %s and %s)", fieldName, foundInDefName, embDef.Name), "")
			}
			foundObj = val
			foundInDefName = embDef.Name
			// Continue checking other embedded structs at the same level for ambiguity
		} else if _, isDirectField := embDef.Fields[fieldName]; isDirectField {
			// Field is defined directly on this embedded struct but not explicitly set in its instance
			if foundObj != nil { // Ambiguity
				return nil, formatErrorWithContext(fset, selPos,
					fmt.Errorf("ambiguous selector %s (found in both %s and %s, as uninitialized field in %s)", fieldName, foundInDefName, embDef.Name, embDef.Name), "")
			}
			foundObj = NULL // Treat as NULL if defined but not set
			foundInDefName = embDef.Name
			// Continue for ambiguity check
		}


		// 2. Recursively search in deeper embedded structs of this embInstance
		if foundObj == nil || foundObj == NULL { // Only recurse if not definitively found or if found as NULL (ambiguity check still needed)
			recursiveFoundObj, err := findFieldInEmbedded(embInstance, fieldName, fset, selPos)
			if err != nil {
				return nil, err // Propagate error (e.g., deeper ambiguity)
			}
			if recursiveFoundObj != nil {
				// If foundObj was NULL and recursiveFoundObj is non-NULL, prefer recursive.
				// If foundObj was non-NULL and recursiveFoundObj is non-NULL, it's an error (handled by recursive call's ambiguity).
				// This logic needs to be careful about how `foundInDefName` is updated for ambiguity messages.
				// For now, let's simplify: if the recursive call finds something concrete, and we haven't, use it.
				// Ambiguity between a direct field of `embDef` and a field from `embDef`'s own embedded struct
				// is handled by Go's shadowing (direct field wins). Our current check order (direct then recursive) mimics this.
				if foundObj != nil && foundObj != NULL && recursiveFoundObj != nil && recursiveFoundObj != NULL {
                     // This implies fieldName is a direct field in `embDef` AND in a struct embedded within `embDef`.
                     // Go's rule: direct field shadows embedded. So `foundObj` (from direct check above) is correct.
                     // The recursive call should not have found it if it was shadowed, unless fieldName is different.
                     // This part of ambiguity (shadowing) is implicitly handled by order of checks.
                     // The primary ambiguity is across different embedded structs at the same level.
				} else if recursiveFoundObj != nil { // recursiveFoundObj could be NULL or a value
					if foundObj != nil && foundObj != NULL && recursiveFoundObj != NULL && recursiveFoundObj != NULL { // Both found non-NULL values at different effective depths through this path
						// This state indicates an error in logic or an unhandled ambiguity.
						// However, the recursive call should handle its own ambiguities.
						// The main concern here is if `fieldName` is found in `embDef1` and also in `embDef2` (siblings).
					}
					if recursiveFoundObj != NULL && foundObj == NULL { // Found concretely in recursion, nothing direct before
						if foundObj != nil { //This means foundObj was already set by another embDef
							return nil, formatErrorWithContext(fset, selPos,
								fmt.Errorf("ambiguous selector %s (found in both %s and deeper in %s)", fieldName, foundInDefName, embDef.Name), "")
						}
						foundObj = recursiveFoundObj
						foundInDefName = embDef.Name // Or a more complex path string
					} else if recursiveFoundObj == NULL && foundObj == nil { // Both are NULL or not found
                        if foundObj != nil { // foundObj was already set by another embDef as NULL
                           // Ambiguity if both paths lead to NULL for a defined field? Go allows this.
                           // We just need one NULL.
                        } else {
						    foundObj = NULL // Propagate the NULL if it's the first time we see it
						    foundInDefName = embDef.Name
                        }
                    } else if recursiveFoundObj != nil && recursiveFoundObj != NULL && foundObj != nil && foundObj != NULL && foundInDefName != embDef.Name {
						// Found in two different peer embedded structs
						return nil, formatErrorWithContext(fset, selPos,
							fmt.Errorf("ambiguous selector %s (found in both %s and %s)", fieldName, foundInDefName, embDef.Name), "")
					}


				}
			}
		}
	}
	return foundObj, nil
}
