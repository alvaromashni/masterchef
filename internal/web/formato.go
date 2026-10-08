package web

import (
	"fmt"
	"strings"
	"time"

	"github.com/alvaromashni/masterchef/internal/store"
)

// Os formatos curtos seguem o design da Fase 6: "há 4 min", "há 2h", "há 5d".

// dataHora mostra a data no fuso local, no formato brasileiro.
func dataHora(t time.Time) string {
	if t.IsZero() {
		return "nunca"
	}
	return t.Local().Format("02/01/2006 15:04")
}

// hora mostra só "15:04" no fuso local.
func hora(t time.Time) string {
	if t.IsZero() {
		return "—"
	}
	return t.Local().Format("15:04")
}

// haQuanto descreve há quanto tempo t aconteceu ("há 5 min", "há 3h").
// Recebe "agora" como parâmetro para ser testável.
func haQuanto(agora, t time.Time) string {
	if t.IsZero() {
		return "nunca"
	}
	if agora.Sub(t) < time.Minute {
		return "agora há pouco"
	}
	return "há " + duracaoCurta(agora.Sub(t))
}

// duracaoCurta escreve uma duração como "5 min", "3h" ou "4d".
func duracaoCurta(d time.Duration) string {
	switch {
	case d < time.Minute:
		return "0 min"
	case d < time.Hour:
		return fmt.Sprintf("%d min", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh", int(d.Hours()))
	default:
		return fmt.Sprintf("%dd", int(d.Hours()/24))
	}
}

var diasDaSemana = []string{"dom", "seg", "ter", "qua", "qui", "sex", "sáb"}
var meses = []string{"jan", "fev", "mar", "abr", "mai", "jun", "jul", "ago", "set", "out", "nov", "dez"}

// diaCurto escreve "6 out".
func diaCurto(t time.Time) string {
	return fmt.Sprintf("%d %s", t.Day(), meses[t.Month()-1])
}

// dataVisita escreve "ter 6 out, 18:20" (usado em "desde a última visita").
func dataVisita(t time.Time) string {
	t = t.Local()
	return fmt.Sprintf("%s %s, %s", diasDaSemana[t.Weekday()], diaCurto(t), t.Format("15:04"))
}

// horaDoEvento escreve "14:31" para hoje, "ontem 19:20" para ontem e
// "6 out" para datas mais antigas.
func horaDoEvento(agora, t time.Time) string {
	agora, t = agora.Local(), t.Local()
	a, m, d := agora.Date()
	hoje := time.Date(a, m, d, 0, 0, 0, 0, agora.Location())
	switch {
	case !t.Before(hoje):
		return t.Format("15:04")
	case !t.Before(hoje.AddDate(0, 0, -1)):
		return "ontem " + t.Format("15:04")
	default:
		return diaCurto(t)
	}
}

// textoRisco é o rótulo do nível de risco na tela.
func textoRisco(nivel string) string {
	switch nivel {
	case "alto":
		return "alto"
	case "medio":
		return "médio"
	case "baixo":
		return "baixo"
	}
	return nivel
}

// refCurta tira o dono do repo: "alvaromashni/api#42" vira "api#42".
func refCurta(ref string) string {
	if i := strings.Index(ref, "/"); i >= 0 && strings.Contains(ref, "#") {
		return ref[i+1:]
	}
	return ref
}

// rotuloEvento é o tipo do evento em palavras ("PR mergeado", "Estado").
func rotuloEvento(e store.Evento) string {
	switch e.Kind {
	case store.EventoIssueCriada:
		return "Issue criada"
	case store.EventoIssueMudouEstado:
		return "Estado"
	case store.EventoPRAberto:
		if strings.HasPrefix(e.Summary, "Reaberto:") {
			return "PR reaberto"
		}
		return "PR aberto"
	case store.EventoPRMergeado:
		return "PR mergeado"
	case store.EventoPRFechado:
		return "PR fechado"
	case store.EventoPRAtualizado:
		return "PR atualizado"
	}
	return e.Kind
}

// prefixosDeResumo são os começos que o coletor grava no resumo. Como a
// tela já mostra o tipo numa coluna própria, o prefixo é tirado na exibição.
var prefixosDeResumo = []string{"Nova issue: ", "Aberto: ", "Reaberto: ", "Mergeado: ", "Fechado sem merge: ", "Atualizado: "}

// textoEvento é o resumo sem o prefixo do tipo.
func textoEvento(e store.Evento) string {
	for _, p := range prefixosDeResumo {
		if strings.HasPrefix(e.Summary, p) {
			if e.Kind == store.EventoPRFechado {
				return strings.TrimPrefix(e.Summary, p) + " (sem merge)"
			}
			return strings.TrimPrefix(e.Summary, p)
		}
	}
	return e.Summary
}

// FalhaSync é a mensagem de erro do último ciclo, separada para a tela.
type FalhaSync struct {
	Quando time.Time // início do ciclo que falhou (zero se não deu para ler)
	Erros  []string  // uma linha por erro
}

// lerFalhaSync separa a mensagem gravada pelo coletor ("Ciclo de <RFC3339>:"
// e uma linha por erro). Mensagem vazia = sync em dia = nil.
func lerFalhaSync(msg string) *FalhaSync {
	if strings.TrimSpace(msg) == "" {
		return nil
	}
	f := &FalhaSync{}
	for _, l := range strings.Split(msg, "\n") {
		l = strings.TrimSpace(l)
		if l == "" {
			continue
		}
		if ts, ok := strings.CutPrefix(l, "Ciclo de "); ok {
			if t, err := time.Parse(time.RFC3339, strings.TrimSuffix(ts, ":")); err == nil {
				f.Quando = t
				continue
			}
		}
		f.Erros = append(f.Erros, l)
	}
	return f
}
