// Package fury es la «furia de IA»: una tercera señal, derivada, de que algo va
// mal con un hilo o una burbuja.
//
// Separada del calor y de la prioridad, y sin tocarlos: el calor dice si la
// realidad cambia, la prioridad cuánto importa, y la furia que algo importante
// se está rompiendo (vencido, inflado, olvidado, sin dueño). No mueve el orden
// por flotabilidad; es un aviso encima.
//
// Se calcula, nunca se guarda ni se pone a mano: se apaga sola cuando el
// problema se arregla. Un sujeto toma el nivel de su regla más grave y dice qué
// reglas dispararon. `now` es un parámetro, como en `internal/heat`: las
// pruebas fijan el reloj.
//
// Las reglas son un catálogo fijo con perillas (umbrales y nivel), que edita el
// lead global. Rechazados: fórmulas libres (fáciles de escribir mal) y reglas
// evaluadas por un LLM (caras y no repetibles). Hilo #6 de Burbujas.
package fury

import (
	"fmt"
	"strings"
	"time"
)

// Niveles. El semáforo de la oficina los pinta verde, amarillo, rojo y rojo
// parpadeando.
const (
	Zen      = 0
	Irritada = 1
	Furiosa  = 2
	Desatada = 3
)

// Level describe un nivel como lo dibuja la UI y lo enciende un semáforo.
type Level struct {
	Level int    `json:"level"`
	Name  string `json:"name"`
	Emoji string `json:"emoji"`
	Color string `json:"color"`
	Blink bool   `json:"blink"`
}

// Levels, del 0 al 3.
var Levels = []Level{
	{Zen, "Zen", "😌", "green", false},
	{Irritada, "Irritada", "😤", "yellow", false},
	{Furiosa, "Furiosa", "😡", "red", false},
	{Desatada, "Desatada", "🔥", "red", true},
}

// Reason es una regla que disparó. En una burbuja, las de sus hilos llevan
// de qué hilo vienen.
type Reason struct {
	Rule       string `json:"rule"`
	Level      int    `json:"level"`
	Text       string `json:"text"`
	Thread     string `json:"thread,omitempty"`
	ThreadName string `json:"thread_name,omitempty"`
}

// Verdict es la furia de un sujeto: el nivel de su regla más grave, y todas
// las que dispararon, de la más grave a la menos.
type Verdict struct {
	Level
	Reasons []Reason `json:"reasons"`
}

// Kind de un sujeto.
const (
	Thread = "thread"
	Bubble = "bubble"
)

// Facts es lo que las reglas miran de un sujeto. Lo arma quien sabe leer la
// base; este paquete no la toca.
type Facts struct {
	Kind     string
	Priority string // P1…P4; en una burbuja, la derivada de impacto × urgencia
	Due      string // AAAA-MM-DD, o vacío
	Done     bool   // hilo completado o burbuja cerrada: no hay furia
	// Responsables: asignados de un hilo, owners de una burbuja.
	People int
	// La banda de calor: hot, dormant, rip o closed.
	Lifecycle string
	// Sólo burbujas: todos sus hilos (también los cerrados, porque mide lo que
	// se ha trabajado), las palabras de todos, los días distintos con evidencia
	// en ellos, y cuándo nació.
	Threads    int
	Words      int
	ActiveDays int
	Created    time.Time
}

// Rules es el catálogo. Cada regla se enciende o se apaga, y dice qué nivel
// produce.
type Rules struct {
	Inflated  InflatedRule  `json:"inflated"`
	Overdue   OverdueRule   `json:"overdue"`
	Forgotten ForgottenRule `json:"forgotten"`
	Ownerless OwnerlessRule `json:"ownerless"`
}

// InflatedRule (R1): una burbuja abierta con mucho trabajo y sin terminar.
// Tiene que pasar TODOS los umbrales; con el doble de cada uno, sube a
// `DoubleLevel`.
type InflatedRule struct {
	On          bool `json:"on"`
	Threads     int  `json:"threads"`
	Words       int  `json:"words"`
	ActiveDays  int  `json:"active_days"`
	AgeDays     int  `json:"age_days"`
	Level       int  `json:"level"`
	DoubleLevel int  `json:"double_level"`
}

// OverdueRule (R2): prioridad alta con el plazo pasado. Más de `LateDays`
// días tarde, sube a `LateLevel`.
type OverdueRule struct {
	On        bool `json:"on"`
	Level     int  `json:"level"`
	LateDays  int  `json:"late_days"`
	LateLevel int  `json:"late_level"`
}

// ForgottenRule (R4): prioridad alta y el calor dice que nadie la toca.
type ForgottenRule struct {
	On           bool `json:"on"`
	DormantLevel int  `json:"dormant_level"`
	RipLevel     int  `json:"rip_level"`
}

// OwnerlessRule (R5): prioridad alta sin nadie que responda.
type OwnerlessRule struct {
	On    bool `json:"on"`
	Level int  `json:"level"`
}

// Defaults son los valores acordados con Angel el 22 oct.
func Defaults() Rules {
	return Rules{
		Inflated:  InflatedRule{On: true, Threads: 8, Words: 6000, ActiveDays: 10, AgeDays: 21, Level: Irritada, DoubleLevel: Furiosa},
		Overdue:   OverdueRule{On: true, Level: Furiosa, LateDays: 7, LateLevel: Desatada},
		Forgotten: ForgottenRule{On: true, DormantLevel: Irritada, RipLevel: Furiosa},
		Ownerless: OwnerlessRule{On: true, Level: Irritada},
	}
}

// Validate rechaza lo que no tiene sentido: niveles fuera de 0–3, umbrales
// negativos o en cero (una regla que dispara con todo no avisa de nada).
func (r Rules) Validate() error {
	lv := func(name string, v int) error {
		if v < Zen || v > Desatada {
			return fmt.Errorf("%s: el nivel va de 0 (Zen) a 3 (Desatada), no %d", name, v)
		}
		return nil
	}
	pos := func(name string, v int) error {
		if v < 1 {
			return fmt.Errorf("%s: el umbral tiene que ser al menos 1", name)
		}
		return nil
	}
	for _, err := range []error{
		lv("burbuja inflada", r.Inflated.Level), lv("burbuja inflada (doble)", r.Inflated.DoubleLevel),
		pos("hilos de la burbuja inflada", r.Inflated.Threads), pos("palabras de la burbuja inflada", r.Inflated.Words),
		pos("días activos de la burbuja inflada", r.Inflated.ActiveDays), pos("edad de la burbuja inflada", r.Inflated.AgeDays),
		lv("vencida", r.Overdue.Level), lv("vencida hace mucho", r.Overdue.LateLevel), pos("días de retraso", r.Overdue.LateDays),
		lv("olvidada (dormida)", r.Forgotten.DormantLevel), lv("olvidada (tumba)", r.Forgotten.RipLevel),
		lv("sin dueño", r.Ownerless.Level),
	} {
		if err != nil {
			return err
		}
	}
	return nil
}

// High: P1 o P2. Lo mismo en hilos y burbujas.
func High(priority string) bool { return priority == "P1" || priority == "P2" }

// Evaluate es la furia de un sujeto en `now`.
func Evaluate(f Facts, r Rules, now time.Time) Verdict {
	v := Verdict{Level: Levels[Zen], Reasons: []Reason{}}
	if f.Done {
		return v
	}
	add := func(rule string, level int, text string) {
		if level > Zen {
			v.Reasons = append(v.Reasons, Reason{Rule: rule, Level: level, Text: text})
		}
	}
	high := High(f.Priority)

	if r.Overdue.On && high && f.Due != "" {
		if late := daysLate(f.Due, now); late > 0 {
			lvl := r.Overdue.Level
			if late > r.Overdue.LateDays {
				lvl = max(lvl, r.Overdue.LateLevel)
			}
			add("overdue", lvl, fmt.Sprintf("vencida hace %s · %s", plural(late, "día", "días"), f.Priority))
		}
	}
	if r.Forgotten.On && high {
		switch f.Lifecycle {
		case "dormant":
			add("forgotten", r.Forgotten.DormantLevel, "olvidada: nadie la toca · "+f.Priority)
		case "rip":
			add("forgotten", r.Forgotten.RipLevel, "olvidada y sin responsable · "+f.Priority)
		}
	}
	if r.Ownerless.On && high && f.People == 0 {
		who := "sin asignados"
		if f.Kind == Bubble {
			who = "sin responsables"
		}
		add("ownerless", r.Ownerless.Level, who+" · "+f.Priority)
	}
	if r.Inflated.On && f.Kind == Bubble {
		i := r.Inflated
		age := 0
		if !f.Created.IsZero() {
			age = int(now.Sub(f.Created).Hours() / 24)
		}
		over := func(k int) bool {
			return f.Threads >= k*i.Threads && f.Words >= k*i.Words && f.ActiveDays >= k*i.ActiveDays && age >= k*i.AgeDays
		}
		if over(1) {
			lvl := i.Level
			if over(2) {
				lvl = max(lvl, i.DoubleLevel)
			}
			add("inflated", lvl, fmt.Sprintf("inflada: %s · %s · %s activos · %s abierta",
				plural(f.Threads, "hilo", "hilos"), plural(f.Words, "palabra", "palabras"),
				plural(f.ActiveDays, "día", "días"), plural(age, "día", "días")))
		}
	}
	return v.settle()
}

// Merge junta la furia propia de una burbuja con la de sus hilos abiertos: el
// máximo, con todas las razones. Las de un hilo dicen de cuál vienen.
func Merge(own Verdict, threads []Verdict, ids, names []string) Verdict {
	out := Verdict{Reasons: append([]Reason{}, own.Reasons...)}
	for i, t := range threads {
		for _, r := range t.Reasons {
			r.Thread, r.ThreadName = ids[i], names[i]
			out.Reasons = append(out.Reasons, r)
		}
	}
	return out.settle()
}

// settle ordena las razones de la más grave a la menos y pone el nivel de la
// primera.
func (v Verdict) settle() Verdict {
	rs := v.Reasons
	for i := 1; i < len(rs); i++ {
		for j := i; j > 0 && rs[j].Level > rs[j-1].Level; j-- {
			rs[j], rs[j-1] = rs[j-1], rs[j]
		}
	}
	v.Level = Levels[Zen]
	if len(rs) > 0 {
		v.Level = Levels[rs[0].Level]
	}
	v.Reasons = rs
	return v
}

// daysLate: días completos desde que venció. Un plazo vence al terminar su
// día; se cuenta en UTC (las zonas por equipo son del diseño largo, hilo #3).
func daysLate(due string, now time.Time) int {
	d, err := time.Parse("2006-01-02", strings.TrimSpace(due)[:min(10, len(strings.TrimSpace(due)))])
	if err != nil {
		return 0
	}
	today := time.Date(now.UTC().Year(), now.UTC().Month(), now.UTC().Day(), 0, 0, 0, 0, time.UTC)
	return int(today.Sub(d).Hours() / 24)
}

func plural(n int, one, many string) string {
	if n == 1 {
		return "1 " + one
	}
	return fmt.Sprintf("%s %s", thousands(n), many)
}

// thousands: 6000 → «6 000», como se escribe aquí.
func thousands(n int) string {
	s := fmt.Sprint(n)
	if n < 10000 && n > -10000 {
		return s
	}
	var b strings.Builder
	for i, c := range s {
		if i > 0 && (len(s)-i)%3 == 0 {
			b.WriteRune(' ')
		}
		b.WriteRune(c)
	}
	return b.String()
}
