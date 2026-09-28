package skywalking

import "fmt"

type apmAttributeFilter struct {
	Resource bool
	Key      string
	Values   *[]string
	Negate   *bool
}

func validateAPMAttributes(filters *[]apmAttributeFilter) error {
	if filters == nil {
		return nil
	}
	if len(*filters) > 32 {
		return fmt.Errorf("APM attribute filter budget exceeded")
	}
	for _, filter := range *filters {
		if filter.Key == "" || len(filter.Key) > 256 {
			return fmt.Errorf("invalid APM attribute key")
		}
		if filter.Values == nil {
			continue
		}
		if len(*filter.Values) < 1 || len(*filter.Values) > 200 {
			return fmt.Errorf("APM attribute values require a bounded nonempty selection")
		}
		for _, v := range *filter.Values {
			if len(v) > 4096 {
				return fmt.Errorf("APM attribute value too long")
			}
		}
	}
	return nil
}

func apmAttributeSQL(filters *[]apmAttributeFilter) (string, []any) {
	where := ""
	args := []any{}
	if filters == nil {
		return where, args
	}
	for _, filter := range *filters {
		if filter.Values == nil {
			continue
		} // null is the typed All selection
		column := "attributes"
		if filter.Resource {
			column = "resource_attributes"
		}
		op := " IN (?)"
		if filter.Negate != nil && *filter.Negate {
			op = " NOT IN (?)"
		}
		where += " AND " + column + "[?]" + op
		args = append(args, filter.Key, *filter.Values)
	}
	return where, args
}
