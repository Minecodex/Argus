package kubernetesinventory

import "k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

// ProjectMutationResult validates and projects Connector responses again before
// they become preview, operation history, or Agent-visible data.
func ProjectMutationResult(definition Definition, input Mutation, result MutationResult, dryRun bool) (MutationResult, error) {
	if result.DryRun != dryRun || dryRun && result.Accepted {
		return MutationResult{}, ErrProtocol
	}
	if err := validateDrainResult(input, result, dryRun); err != nil {
		return MutationResult{}, err
	}
	for index, object := range []*Object{result.Before, result.After} {
		if object == nil {
			continue
		}
		if object.Definition != definition || object.Name != input.Query.Name || object.Namespace != input.Query.Namespace || definition.Namespaced && object.NamespaceUID != input.Query.NamespaceUID || input.Query.UID != "" && object.UID != input.Query.UID {
			return MutationResult{}, ErrIdentityChanged
		}
		var data unstructured.Unstructured
		if data.UnmarshalJSON(object.Data) != nil || string(data.GetUID()) != object.UID || data.GetResourceVersion() != object.ResourceVersion || data.GetName() != object.Name || data.GetNamespace() != object.Namespace {
			return MutationResult{}, ErrProtocol
		}
		projected, err := projectObject(definition, data, object.NamespaceUID, object.ObservedAt, index == 0 || !dryRun)
		if err != nil {
			return MutationResult{}, err
		}
		projected.Redacted = projected.Redacted || object.Redacted
		if index == 0 {
			result.Before = &projected
		} else {
			result.After = &projected
		}
	}
	return result, nil
}
