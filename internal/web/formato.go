package web

import (
	"fmt"
	"time"
)

// dataHora mostra a data no fuso local, no formato brasileiro.
func dataHora(t time.Time) string {
	if t.IsZero() {
		return "nunca"
	}
	return t.Local().Format("02/01/2006 15:04")
}

// haQuanto descreve há quanto tempo t aconteceu, de forma curta ("há 5 min").
// Recebe "agora" como parâmetro para ser testável.
func haQuanto(agora, t time.Time) string {
	if t.IsZero() {
		return "nunca"
	}
	d := agora.Sub(t)
	switch {
	case d < time.Minute:
		return "agora há pouco"
	case d < time.Hour:
		return fmt.Sprintf("há %d min", int(d.Minutes()))
	case d < 48*time.Hour:
		return fmt.Sprintf("há %d h", int(d.Hours()))
	default:
		return fmt.Sprintf("há %d dias", int(d.Hours()/24))
	}
}

// textoRisco é o rótulo do nível de risco na tela.
func textoRisco(nivel string) string {
	switch nivel {
	case "alto":
		return "Risco alto"
	case "medio":
		return "Risco médio"
	case "baixo":
		return "Risco baixo"
	}
	return nivel
}
