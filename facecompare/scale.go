package facecompare

import "github.com/rodfileto/trackid/cluster"

// ScaleLevel is a level of the opinion scale (cluster.Conclusion) as the lab and the laudo word it.
type ScaleLevel struct {
	Code      cluster.Conclusion `json:"code"`
	EN        string             `json:"en"`
	PT        string             `json:"pt"`
	MeaningPT string             `json:"meaningPt"`
}

// Scale is the opinion scale: the five categories of OSAC 2022-S-0001, which FISWG refers to. The
// meanings are TrackID's, written after OSAC's definitions.
var Scale = []ScaleLevel{
	{cluster.ConclusionStrongSupportCommon, "Strong support for common source", "Suporte forte à origem comum",
		"as semelhanças observadas superam em muito as diferenças; as observações são muito mais prováveis se as imagens retratarem a mesma pessoa do que pessoas distintas."},
	{cluster.ConclusionSupportCommon, "Support for common source", "Suporte à origem comum",
		"as semelhanças observadas superam as diferenças, sem alcançar o suporte forte; as observações são mais prováveis se as imagens retratarem a mesma pessoa do que pessoas distintas."},
	{cluster.ConclusionInconclusive, "Inconclusive", "Inconclusivo",
		"as informações disponíveis não permitem favorecer nenhuma das hipóteses; as observações são igualmente prováveis sob ambas."},
	{cluster.ConclusionSupportDifferent, "Support for different sources", "Suporte a origens distintas",
		"as diferenças observadas superam as semelhanças, sem alcançar o suporte forte; as observações são mais prováveis se as imagens retratarem pessoas distintas."},
	{cluster.ConclusionStrongSupportDifferent, "Strong support for different sources", "Suporte forte a origens distintas",
		"as diferenças observadas superam em muito as semelhanças, ou não se observam semelhanças distintivas; as observações são muito mais prováveis se as imagens retratarem pessoas distintas."},
}

// Level returns c's level of the scale.
func Level(c cluster.Conclusion) (ScaleLevel, bool) {
	for _, l := range Scale {
		if l.Code == c {
			return l, true
		}
	}
	return ScaleLevel{}, false
}
