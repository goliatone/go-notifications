package i18n

import (
	"bytes"
	"strings"
	"testing"
	"text/template"
	"time"
)

func TestTranslatorAndFormatterIntegrationForEsMX(t *testing.T) {
	store := NewStaticStore(Translations{
		"es": newStringCatalog("es", map[string]string{
			"receipt.heading": "Resumen de pedido para %s",
			"receipt.summary": "Tu pedido incluye %d artículos",
		}),
	})

	cfg, err := NewConfig(
		WithStore(store),
		WithLocales("es", "es-MX"),
		WithDefaultLocale("es"),
		WithFallback("es-MX", "es"),
		WithFormatterLocales("es", "es-MX"),
	)
	if err != nil {
		t.Fatalf("NewConfig: %v", err)
	}

	translator, err := cfg.BuildTranslator()
	if err != nil {
		t.Fatalf("BuildTranslator: %v", err)
	}

	helpers := cfg.TemplateHelpers(translator, HelperConfig{
		TemplateHelperKey: "t",
	})

	tmpl := template.Must(template.New("integration").Funcs(helpers).Parse(`
{{$locale := .Locale}}
{{t $locale "receipt.heading" .Customer}}
Fecha: {{format_date $locale .OrderDate}}
Lista: {{format_list $locale .Items}}
Total: {{format_currency $locale .Total "MXN"}}
Conteo: {{format_number $locale .ItemCount 0}}
`))

	var buf bytes.Buffer
	err = tmpl.Execute(&buf, struct {
		Locale    string
		Customer  string
		OrderDate time.Time
		Items     []string
		Total     float64
		ItemCount float64
	}{
		Locale:    "es-MX",
		Customer:  "Lucía",
		OrderDate: time.Date(2024, time.May, 15, 0, 0, 0, 0, time.UTC),
		Items:     []string{"uno", "dos", "tres"},
		Total:     1234.56,
		ItemCount: 3,
	})
	if err != nil {
		t.Fatalf("execute template: %v", err)
	}

	output := strings.TrimSpace(buf.String())

	assertContains := func(substr string) {
		if !strings.Contains(output, substr) {
			t.Fatalf("expected output to include %q, got:\n%s", substr, output)
		}
	}

	assertContains("Resumen de pedido para Lucía")
	assertContains("Fecha: 15 de mayo de 2024")
	assertContains("Lista: uno, dos y tres")
	assertContains("Total: 1.234,56 $") // $ is the symbol for MXN
	assertContains("Conteo: 3")
}
