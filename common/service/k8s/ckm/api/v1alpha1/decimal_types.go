package v1alpha1

import "github.com/shopspring/decimal"

// Decimal wraps shopspring/decimal so CRD types can use exact decimal prices.
// +kubebuilder:validation:Type=string
// +kubebuilder:validation:Format=decimal
type Decimal struct {
	Decimal decimal.Decimal `json:"-"`
}

func NewDecimalFromInt(v int64) Decimal {
	return Decimal{Decimal: decimal.NewFromInt(v)}
}

func (in *Decimal) DeepCopyInto(out *Decimal) {
	if in == nil || out == nil {
		return
	}
	out.Decimal = in.Decimal.Copy()
}

func (in *Decimal) DeepCopy() *Decimal {
	if in == nil {
		return nil
	}
	out := new(Decimal)
	in.DeepCopyInto(out)
	return out
}

func (Decimal) OpenAPISchemaType() []string {
	return []string{"string"}
}

func (Decimal) OpenAPISchemaFormat() string {
	return "decimal"
}

func (d Decimal) MarshalJSON() ([]byte, error) {
	return d.Decimal.MarshalJSON()
}

func (d *Decimal) UnmarshalJSON(data []byte) error {
	if d == nil {
		return nil
	}
	return d.Decimal.UnmarshalJSON(data)
}
