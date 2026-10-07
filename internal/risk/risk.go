// Package risk é o motor de risco: uma função pura que olha os arquivos de
// um PR e as regras do config.yaml e diz o nível de risco e o porquê.
//
// "Pura" quer dizer: sem banco, sem rede, sem relógio. A mesma entrada dá
// sempre a mesma saída, o que deixa o motor fácil de testar e de confiar.
package risk

import (
	"fmt"

	"github.com/bmatcuk/doublestar/v4"

	"github.com/alvaromashni/masterchef/internal/config"
)

// Níveis possíveis, do mais grave para o menos grave.
const (
	Alto  = "alto"
	Medio = "medio"
	Baixo = "baixo"
)

// Resultado é o nível de risco e a lista de motivos que levaram a ele.
// Motivos nunca vem vazio para alto/medio: o revisor precisa saber o porquê.
type Resultado struct {
	Nivel   string
	Motivos []string
}

// Avaliar aplica as regras da seção 8 do CONTEXT.md:
//
//   - alto se algum arquivo casa com uma regra "alto";
//   - senão medio se casa com regra "medio", se o diff passa do limite
//     ou se nenhum arquivo casa com os padrões de teste;
//   - senão baixo.
//
// Todos os motivos encontrados entram na lista (inclusive os de nível medio
// num PR alto), porque cada um é algo que o revisor deve olhar.
func Avaliar(arquivos []string, linhasAlteradas int, cfg config.Risco) Resultado {
	r := Resultado{Nivel: Baixo}

	for _, regra := range cfg.Regras {
		arquivo, casou := primeiroQueCasa(regra.Padrao, arquivos)
		if !casou {
			continue
		}
		// Mostrar um arquivo de exemplo ajuda o revisor a ir direto ao ponto.
		r.Motivos = append(r.Motivos, fmt.Sprintf("%s (%s)", regra.Motivo, arquivo))
		r.subirPara(regra.Nivel)
	}

	if linhasAlteradas > cfg.LimiteLinhasDiff {
		r.Motivos = append(r.Motivos, fmt.Sprintf("Diff grande: %d linhas alteradas (limite %d)", linhasAlteradas, cfg.LimiteLinhasDiff))
		r.subirPara(Medio)
	}

	// Sem padrões de teste configurados não dá para dizer se há testes,
	// então a regra só vale quando a lista existe.
	if len(cfg.PadroesDeTeste) > 0 && !algumCasa(cfg.PadroesDeTeste, arquivos) {
		r.Motivos = append(r.Motivos, "Nenhum arquivo de teste alterado")
		r.subirPara(Medio)
	}

	return r
}

// subirPara aumenta o nível, mas nunca o diminui (um "medio" não apaga um "alto").
func (r *Resultado) subirPara(nivel string) {
	if Peso(nivel) > Peso(r.Nivel) {
		r.Nivel = nivel
	}
}

// Peso transforma o nível num número para comparar. Também é usado para
// ordenar a fila de review (alto primeiro).
func Peso(nivel string) int {
	switch nivel {
	case Alto:
		return 3
	case Medio:
		return 2
	case Baixo:
		return 1
	}
	return 0
}

func primeiroQueCasa(padrao string, arquivos []string) (string, bool) {
	for _, a := range arquivos {
		// Os padrões já foram validados na carga da config, então o erro
		// (padrão malformado) não acontece aqui; tratamos como "não casou".
		if ok, err := doublestar.Match(padrao, a); err == nil && ok {
			return a, true
		}
	}
	return "", false
}

func algumCasa(padroes, arquivos []string) bool {
	for _, p := range padroes {
		if _, ok := primeiroQueCasa(p, arquivos); ok {
			return true
		}
	}
	return false
}
