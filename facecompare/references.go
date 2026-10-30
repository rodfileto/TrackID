package facecompare

// Reference is a document the lab follows, cited by title, version and the sections used.
type Reference struct {
	Code     string `json:"code"`
	Title    string `json:"title"`
	Version  string `json:"version"`
	Sections string `json:"sections"`
	Use      string `json:"use"` // what the lab takes from it, in Portuguese
}

// References lists the documents the comparison follows; the record cites them, and the lab shows
// them. They are FISWG's documents (fiswg.org/documents), as published, and the OSAC standard
// FISWG points to for the opinion scale.
var References = []Reference{
	{"METHODOLOGY", "FISWG, Facial Comparison Overview and Methodology Guidelines", "2.1", "§5–7",
		"análise morfológica como método; superposição apenas em conjunto com ela e com o mesmo ponto de vista; fotoantropometria não utilizada"},
	{"ACEV", "FISWG, Guideline for the Use of ACE-V Methodology in One-to-One Examinations", "1.0 (2023)", "§5",
		"etapas de análise, comparação, avaliação e verificação; imagem questionada analisada antes da padrão; vocabulário da opinião"},
	{"FEATURES", "FISWG, Facial Image Comparison Feature List for Morphological Analysis (ASTM E3149)", "2.0 (2018)", "§4",
		"componentes faciais e características comparadas"},
	{"STABILITY", "FISWG, Physical Stability of Facial Features of Adults", "2.0 (2021)", "tabelas, p. 5–14",
		"estabilidade das características sob expressão, tempo, peso, saúde e alterações intencionais"},
	{"MARKUPS", "FISWG, Facial Image Comparison Best Practices for Markups & Annotations", "1.0 (2021)", "§4–5",
		"tipos de marcação permitidos; imagem sem marcações anexada; termos a evitar"},
	{"PROCESSING", "FISWG, Image Processing Techniques for Facial Image Comparison", "1.0 (2022)", "§4–6",
		"processamento mínimo, global e reprodutível, registrado com o software e sua versão"},
	{"FACTORS", "FISWG, Image Factors to Consider in Facial Image Comparison", "1.0 (2021)", "§4–13",
		"fatores de imagem avaliados na análise"},
	{"DOCUMENTATION", "FISWG, Minimum Guidelines for Facial Image Comparison Documentation", "2.0 (2022)", "§5–9",
		"conteúdo do registro"},
	{"MANIPULATION", "FISWG, Visual Image Manipulation Detection in Facial Images", "1.0 (2024)", "§4–5",
		"registro de suspeita de manipulação e encaminhamento a perito em imagem digital"},
	{"OSAC", "OSAC 2022-S-0001, Standard Guide for Image Comparison Opinions", "proposta", "§4–5",
		"categorias da escala de opinião, referida pelo FISWG"},
}
