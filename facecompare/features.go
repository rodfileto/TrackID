// Package facecompare is the one-to-one facial comparison the face lab offers
// (docs/facial-comparison-tool-plan.md): the feature list a morphological analysis follows, the
// stability of each feature, alignment by a similarity transform, image metrics for the analysis
// step, markups, findings, an opinion on the OSAC scale, the wording check, the comparison sheet
// and the record in Portuguese.
//
// The structure follows FISWG's documents, which are cited, not copied: the components and
// characteristics are named after the Facial Image Comparison Feature List for Morphological
// Analysis (v2.0, ASTM E3149), and the stability ratings are transcribed from Physical Stability
// of Facial Features of Adults (v2.0). Only the names and the H/M/L ratings are kept here, with
// the page each comes from; the documents' descriptive text is not reproduced.
package facecompare

// Stability is how much a characteristic can change in the same person under a factor: High
// (little to no change), Medium (moderate) or Low (substantial).
type Stability string

const (
	High   Stability = "H"
	Medium Stability = "M"
	Low    Stability = "L"
)

// Factors under which FISWG rates stability, in its tables' order.
const (
	FactorExpression = "EXPRESSION"
	FactorShortTerm  = "SHORT_TERM" // time-related changes, short term
	FactorLongTerm   = "LONG_TERM"  // time-related changes, long term
	FactorWeight     = "WEIGHT"     // marked weight change
	FactorHealth     = "HEALTH"     // changes in health
	FactorAlteration = "ALTERATION" // intentional alterations
)

// StabilityFactors lists the factors in order, with their labels.
var StabilityFactors = []Label{
	{FactorExpression, "Expression", "Expressão"},
	{FactorShortTerm, "Time, short term", "Tempo (curto prazo)"},
	{FactorLongTerm, "Time, long term", "Tempo (longo prazo)"},
	{FactorWeight, "Marked weight change", "Variação acentuada de peso"},
	{FactorHealth, "Changes in health", "Alterações de saúde"},
	{FactorAlteration, "Intentional alterations", "Alterações intencionais"},
}

// Label is a code with its English and Portuguese names.
type Label struct {
	Code string `json:"code"`
	EN   string `json:"en"`
	PT   string `json:"pt"`
}

// Ratings is a characteristic's stability under each factor, from the stability document's page.
type Ratings struct {
	Expression Stability `json:"EXPRESSION"`
	ShortTerm  Stability `json:"SHORT_TERM"`
	LongTerm   Stability `json:"LONG_TERM"`
	Weight     Stability `json:"WEIGHT"`
	Health     Stability `json:"HEALTH"`
	Alteration Stability `json:"ALTERATION"`
	Page       int       `json:"page"` // the page of FISWG's Physical Stability of Facial Features of Adults v2.0
}

// Under is the rating under factor f.
func (r Ratings) Under(f string) Stability {
	switch f {
	case FactorExpression:
		return r.Expression
	case FactorShortTerm:
		return r.ShortTerm
	case FactorLongTerm:
		return r.LongTerm
	case FactorWeight:
		return r.Weight
	case FactorHealth:
		return r.Health
	case FactorAlteration:
		return r.Alteration
	}
	return ""
}

// Characteristic is a component characteristic: what a finding is about. Sided ones are recorded
// for the subject's right and left separately.
type Characteristic struct {
	Code        string   `json:"code"`
	EN          string   `json:"en"`
	PT          string   `json:"pt"`
	Sided       bool     `json:"sided,omitempty"`
	Descriptors []string `json:"descriptors"` // codes in Descriptors
	Stability   *Ratings `json:"stability,omitempty"`
}

// Component is one of the feature list's nineteen facial components, top of the face to bottom.
type Component struct {
	Code            string           `json:"code"`
	EN              string           `json:"en"`
	PT              string           `json:"pt"`
	Characteristics []Characteristic `json:"characteristics"`
}

// Sources the feature list and the stability ratings are taken from.
const (
	FeatureListSource = "FISWG, Facial Image Comparison Feature List for Morphological Analysis, v2.0 (2018); ASTM E3149"
	StabilitySource   = "FISWG, Physical Stability of Facial Features of Adults, v2.0 (2021)"
)

func r(e, st, lt, w, h, a Stability, page int) *Ratings {
	return &Ratings{e, st, lt, w, h, a, page}
}

func c(code, en, pt string, rt *Ratings, descriptors ...string) Characteristic {
	return Characteristic{Code: code, EN: en, PT: pt, Descriptors: descriptors, Stability: rt}
}

func sided(ch Characteristic) Characteristic {
	ch.Sided = true
	return ch
}

const H, M, L = High, Medium, Low

// Descriptors names the characteristic descriptors used below.
var Descriptors = map[string]Label{}

func init() {
	for _, d := range []Label{
		{"TEXTURE", "Texture", "Textura"}, {"TONE", "Tone/colour", "Tom/cor"},
		{"PORTRAIT_CONTOUR", "Frontal contour", "Contorno frontal"}, {"PROFILE_CONTOUR", "Profile contour", "Contorno de perfil"},
		{"NOSE_VS_EYES", "Nose width relative to eye distances", "Largura do nariz relativa às distâncias dos olhos"},
		{"MOUTH_VS_EYES", "Mouth width relative to eye distances", "Largura da boca relativa às distâncias dos olhos"},
		{"NOSE_VS_MOUTH", "Nose width relative to mouth", "Largura do nariz relativa à boca"},
		{"NOSE_LIP_VS_FACE", "Nose to upper lip relative to face length", "Nariz a lábio superior relativo ao comprimento da face"},
		{"CHIN_LIP_VS_FACE", "Chin to lower lip relative to face length", "Queixo a lábio inferior relativo ao comprimento da face"},
		{"EAR_POSITION", "Ear position relative to eyes, nose and mouth", "Posição das orelhas relativa a olhos, nariz e boca"},
		{"EYE_POSITION", "Eye position relative to face length", "Posição dos olhos relativa ao comprimento da face"},
		{"SHAPE", "Shape", "Forma"}, {"SIZE", "Size", "Tamanho"}, {"POSITION", "Position", "Posição"},
		{"SYMMETRY", "Symmetry", "Simetria"}, {"PROMINENCE", "Prominence", "Proeminência"},
		{"DISTRIBUTION", "Shape/spatial distribution", "Forma/distribuição espacial"},
		{"DENSITY", "Density and its distribution", "Densidade e sua distribuição"},
		{"HEIGHT", "Relative height", "Altura relativa"}, {"WIDTH", "Relative width", "Largura relativa"},
		{"LENGTH", "Length", "Comprimento"}, {"SLOPE", "Slope/contour (profile)", "Inclinação/contorno (perfil)"},
		{"CONTINUITY", "Continuity", "Continuidade"}, {"TILT", "Tilt", "Inclinação"},
		{"END_POSITIONS", "End positions relative to the canthi", "Posição das extremidades relativa aos cantos dos olhos"},
		{"HAIR_DETAILS", "Hair details", "Detalhes dos pelos"}, {"LONGER_HAIRS", "Noticeably longer hairs", "Pelos notadamente mais longos"},
		{"ANGLE", "Angle", "Ângulo"}, {"CREASE", "Crease visibility", "Visibilidade do sulco"},
		{"IRIS_RELATION", "Position relative to iris/pupil", "Posição relativa à íris/pupila"},
		{"LASHES", "Lash characteristics", "Características dos cílios"}, {"PROTRUSION", "Protrusion", "Protrusão"},
		{"VESSELS", "Visible vessels and defects", "Vasos visíveis e defeitos"}, {"COLOUR", "Colour", "Cor"},
		{"VISIBILITY", "Visibility", "Visibilidade"}, {"PUPIL", "Pupil irregularity", "Irregularidade da pupila"},
		{"CARUNCLE", "Caruncle", "Carúncula"}, {"OFFSET", "Offset (one higher)", "Desnível (um mais alto)"},
		{"DIMPLE", "Dimple", "Covinha"}, {"DEPTH", "Depth", "Profundidade"}, {"BIFID", "Bifid", "Bífido"},
		{"DEVIATION", "Deviation to a side", "Desvio lateral"}, {"THICKNESS", "Thickness", "Espessura"},
		{"HAIR", "Hair", "Pelos"}, {"QUANTITY", "Quantity", "Quantidade"}, {"ATTACHMENT", "Attached or detached", "Aderido ou solto"},
		{"FULLNESS", "Fullness", "Volume"}, {"VERMILION", "Vermilion border", "Contorno do vermelhão"},
		{"DETAIL", "Details", "Detalhes"}, {"CONTACT", "Degree of contact", "Grau de contato"},
		{"COMMISSURES", "Corners of the mouth", "Comissuras labiais"}, {"DEGREE", "Degree", "Grau"},
		{"ALIGNMENT", "Alignment/position", "Alinhamento/posição"}, {"CONDITION", "Condition", "Condição"},
		{"DEFINITION", "Definition", "Definição"}, {"ORIENTATION", "Orientation", "Orientação"},
		{"EDGE", "Outline/edge definition", "Definição do contorno"}, {"PATTERN", "Pattern", "Padrão"},
		{"LOCATION", "Location", "Localização"}, {"DESCRIPTION", "Description", "Descrição"},
	} {
		Descriptors[d.Code] = d
	}
}

// Components is the feature list.
var Components = []Component{
	{"SKIN", "Skin", "Pele", []Characteristic{
		c("SKIN_OVERALL", "Overall skin appearance", "Aparência geral da pele", r(M, M, L, M, L, L, 5), "TEXTURE", "TONE"),
	}},
	{"OUTLINE", "Face/head outline", "Contorno da face/cabeça", []Characteristic{
		c("CRANIAL_VAULT", "Shape of cranial vault", "Forma da calota craniana", r(H, H, H, M, H, M, 5), "PORTRAIT_CONTOUR", "PROFILE_CONTOUR"),
		c("FACE_SHAPE", "Overall shape of face", "Forma geral da face", r(L, H, M, L, L, L, 5), "PORTRAIT_CONTOUR", "PROFILE_CONTOUR"),
	}},
	{"COMPOSITION", "Face/head composition", "Composição da face/cabeça", []Characteristic{
		c("PROPORTIONS", "Proportions/position of features on face", "Proporções/posição dos componentes na face", r(L, H, M, M, L, L, 5),
			"NOSE_VS_EYES", "MOUTH_VS_EYES", "NOSE_VS_MOUTH", "NOSE_LIP_VS_FACE", "CHIN_LIP_VS_FACE", "EAR_POSITION", "EYE_POSITION"),
	}},
	{"HAIR", "Hair", "Cabelo", []Characteristic{
		c("HAIR", "Hair", "Cabelo", r(H, L, L, H, L, L, 6), "DISTRIBUTION", "TEXTURE", "SYMMETRY", "DENSITY", "TONE"),
		c("FOREHEAD_HAIRLINE", "Forehead hairline", "Linha frontal do cabelo", r(H, L, L, H, L, L, 6), "SHAPE", "SYMMETRY"),
		sided(c("SIDE_HAIRLINE", "Hairline, side", "Linha lateral do cabelo", r(H, L, L, H, L, L, 6), "SHAPE")),
		c("BALDNESS", "Cranial baldness pattern", "Padrão de calvície", r(H, L, L, H, L, L, 6), "SHAPE", "DISTRIBUTION"),
	}},
	{"FOREHEAD", "Forehead", "Testa", []Characteristic{
		c("FOREHEAD_SHAPE", "Forehead shape", "Forma da testa", r(H, H, M, M, H, L, 6), "HEIGHT", "WIDTH", "SLOPE"),
		c("BROW_RIDGES", "Brow ridges", "Arcos superciliares", r(H, H, M, M, M, L, 6), "PROMINENCE", "CONTINUITY"),
	}},
	{"EYEBROWS", "Eyebrows", "Sobrancelhas", []Characteristic{
		sided(c("EYEBROW", "Eyebrow", "Sobrancelha", r(L, H, L, H, L, L, 6), "SHAPE", "SIZE", "TILT", "END_POSITIONS", "DENSITY", "HAIR_DETAILS", "LONGER_HAIRS")),
		c("UNIBROW", "Conjoined eyebrows", "Sobrancelhas unidas", nil, "PRESENCE"),
		c("EYEBROW_ASYMMETRY", "Asymmetry between eyebrows", "Assimetria entre as sobrancelhas", r(L, H, H, H, L, L, 6), "SHAPE", "SIZE", "POSITION", "HAIR_DETAILS"),
	}},
	{"EYES", "Eyes", "Olhos", []Characteristic{
		c("INTERCANTHAL", "Intercanthal distance", "Distância intercantal", r(H, H, H, H, H, H, 7), "WIDTH"),
		c("INTERPUPILLARY", "Interpupillary distance", "Distância interpupilar", r(M, H, H, H, M, L, 7), "WIDTH"),
		sided(c("FISSURE", "Eye fissure opening (outline)", "Fenda palpebral (contorno)", r(L, H, M, M, L, M, 7), "SHAPE", "ANGLE")),
		sided(c("UPPER_EYELID", "Upper eyelid, with lashes", "Pálpebra superior, com cílios", r(L, H, M, M, L, L, 7), "PROMINENCE", "PROTRUSION", "CREASE", "IRIS_RELATION", "LASHES")),
		sided(c("LOWER_EYELID", "Lower eyelid, with lashes", "Pálpebra inferior, com cílios", r(L, H, L, M, L, L, 7), "PROMINENCE", "PROTRUSION", "CREASE", "IRIS_RELATION", "LASHES")),
		sided(c("EYEBALL", "Eyeball prominence", "Proeminência do globo ocular", r(H, H, H, M, L, M, 7), "PROMINENCE")),
		sided(c("SCLERA", "Sclera", "Esclera", r(H, M, M, H, L, L, 7), "VESSELS", "COLOUR")),
		sided(c("IRIS", "Iris", "Íris", r(L, H, M, H, L, L, 7), "COLOUR", "VISIBILITY", "SIZE", "POSITION", "PUPIL")),
		sided(c("MEDIAL_CANTHUS", "Medial canthus", "Canto medial", r(H, H, H, H, L, M, 7), "CARUNCLE", "SHAPE", "ANGLE")),
		sided(c("LATERAL_CANTHUS", "Lateral canthus", "Canto lateral", r(H, H, M, H, L, M, 7), "SHAPE", "ANGLE")),
		c("EYE_ASYMMETRY", "Asymmetry between eyes", "Assimetria entre os olhos", r(H, H, H, H, L, M, 7), "SHAPE", "OFFSET", "COLOUR", "IRIS_RELATION"),
	}},
	{"CHEEKS", "Cheeks", "Bochechas", []Characteristic{
		sided(c("CHEEKBONE", "Cheekbone", "Maçã do rosto (zigomático)", r(H, H, M, L, L, L, 8), "PROMINENCE")),
		sided(c("CHEEK_SHAPE", "Cheek shape (soft tissue)", "Forma da bochecha (tecido mole)", r(L, M, L, L, L, L, 8), "SHAPE", "DIMPLE")),
	}},
	{"NOSE", "Nose", "Nariz", []Characteristic{
		c("NASAL_OUTLINE", "Nasal outline (profile/front)", "Contorno nasal (perfil/frente)", r(L, H, M, M, M, L, 8), "SHAPE", "LENGTH", "WIDTH", "PROMINENCE", "SYMMETRY"),
		c("NASAL_ROOT", "Nasal root (bridge)", "Raiz nasal (dorso alto)", r(H, H, H, H, H, L, 8), "WIDTH", "LENGTH", "SHAPE", "DEPTH", "ANGLE"),
		c("NASAL_BODY", "Nasal body", "Corpo nasal (dorso)", r(M, H, H, M, H, L, 8), "WIDTH", "LENGTH", "SHAPE", "ANGLE", "PROFILE_CONTOUR"),
		c("NASAL_TIP", "Nasal tip", "Ponta nasal", r(L, H, L, M, M, L, 8), "SHAPE", "BIFID", "ANGLE", "SYMMETRY"),
		c("NASAL_BASE", "Nasal base", "Base nasal", r(L, H, M, M, M, L, 8), "WIDTH", "HEIGHT", "DEVIATION"),
		c("ALAE", "Alae (wings of nose)", "Asas do nariz", r(L, H, M, M, M, L, 8), "THICKNESS", "SYMMETRY", "SHAPE"),
		c("NOSTRILS", "Nostrils", "Narinas", r(L, H, M, M, M, L, 8), "SHAPE", "SIZE", "SYMMETRY", "HAIR"),
		c("COLUMELLA", "Columella", "Columela", r(M, H, M, H, M, L, 8), "WIDTH", "LENGTH", "POSITION", "SYMMETRY"),
	}},
	{"EARS", "Ears", "Orelhas", []Characteristic{
		c("EAR_ASYMMETRY", "Asymmetry between ears", "Assimetria entre as orelhas", r(M, H, H, H, H, L, 9), "SIZE", "SHAPE", "PROTRUSION", "POSITION"),
		sided(c("EAR_PROTRUSION", "Ear protrusion", "Protrusão da orelha", r(M, H, M, L, H, L, 9), "DEGREE")),
		sided(c("EAR_OVERALL", "Overall ear", "Orelha, conjunto", r(M, H, M, M, H, L, 9), "SIZE", "SHAPE", "ANGLE")),
		sided(c("HELIX", "Helix (superior, inferior)", "Hélice (superior, inferior)", r(H, H, H, H, H, L, 9), "SIZE", "SHAPE")),
		sided(c("TUBERCLE", "Auricular (Darwin's) tubercle", "Tubérculo auricular (de Darwin)", r(H, H, H, H, H, L, 9), "SIZE", "SHAPE", "QUANTITY")),
		sided(c("ANTIHELIX", "Antihelix", "Anti-hélice", r(H, H, H, H, H, L, 9), "SIZE", "SHAPE")),
		sided(c("CRURA", "Crura of antihelix", "Ramos da anti-hélice", r(H, H, H, H, H, M, 9), "SIZE", "SHAPE")),
		sided(c("TRIANGULAR_FOSSA", "Triangular fossa", "Fossa triangular", r(H, H, H, H, H, M, 9), "SIZE", "SHAPE")),
		sided(c("CRUS_HELIX", "Crus of helix", "Ramo da hélice", r(H, H, H, H, H, M, 9), "SIZE", "SHAPE")),
		sided(c("SCAPHOID_FOSSA", "Scaphoid fossa", "Fossa escafoide", r(H, H, H, H, H, M, 9), "SIZE", "SHAPE")),
		sided(c("CONCHA", "Concha (superior, inferior)", "Concha (superior, inferior)", r(H, H, H, H, H, M, 9), "SIZE", "SHAPE")),
		sided(c("TRAGUS", "Tragus", "Trago", r(H, H, H, H, H, L, 9), "SIZE", "SHAPE", "PROTRUSION")),
		sided(c("ANTITRAGUS", "Antitragus", "Antitrago", r(H, H, H, H, H, L, 9), "SIZE", "SHAPE", "PROTRUSION")),
		sided(c("INTERTRAGIC_NOTCH", "Intertragic notch", "Incisura intertrágica", r(H, H, H, H, H, M, 9), "SIZE", "SHAPE")),
		sided(c("ANTERIOR_KNOB", "Anterior knob", "Nódulo anterior", r(H, H, H, H, H, L, 9), "SIZE", "SHAPE")),
		sided(c("ANTERIOR_NOTCH", "Anterior notch", "Incisura anterior", r(H, H, H, H, H, M, 9), "SIZE", "SHAPE")),
		sided(c("POSTERIOR_FURROW", "Posterior auricular furrow", "Sulco auricular posterior", r(H, H, H, H, H, M, 10), "SIZE", "SHAPE")),
		sided(c("LOBULE", "Lobule (lobe)", "Lóbulo", r(H, H, M, M, H, L, 10), "SIZE", "SHAPE", "ATTACHMENT")),
		c("EAR_ABNORMALITY", "Ear abnormalities", "Anomalias da orelha", r(H, H, M, H, H, L, 10), "DESCRIPTION"),
	}},
	{"MOUTH", "Mouth", "Boca", []Characteristic{
		c("PHILTRUM", "Philtrum", "Filtro labial", r(L, H, L, M, M, L, 10), "PROMINENCE", "WIDTH", "DEPTH", "SYMMETRY"),
		c("MOUTH_OVERALL", "Overall mouth", "Boca, conjunto", r(L, H, L, M, M, L, 10), "SHAPE", "SYMMETRY"),
		c("UPPER_LIP", "Upper lip", "Lábio superior", r(L, H, L, M, M, L, 10), "SHAPE", "FULLNESS", "PROTRUSION", "SYMMETRY", "VERMILION", "DETAIL"),
		c("LOWER_LIP", "Lower lip", "Lábio inferior", r(L, H, L, M, M, L, 10), "SHAPE", "FULLNESS", "PROTRUSION", "SYMMETRY", "VERMILION", "DETAIL"),
		c("LIP_FISSURE", "Lip fissure", "Rima labial", r(L, M, L, M, M, L, 10), "SHAPE", "SYMMETRY", "CONTACT", "COMMISSURES"),
		c("MOUTH_ASYMMETRY", "Mouth asymmetry", "Assimetria da boca", r(L, H, H, H, M, L, 10), "SYMMETRY"),
		c("OCCLUSION", "Overall dental occlusion", "Oclusão dentária", r(L, M, M, H, M, L, 10), "SYMMETRY", "CONTACT"),
		c("GNATHISM", "Gnathism", "Gnatismo", r(H, H, M, M, M, L, 10), "DESCRIPTION", "DEGREE"),
		c("TEETH", "Characteristic detail of teeth", "Detalhes característicos dos dentes", r(H, H, M, H, M, L, 10), "SHAPE", "SIZE", "ALIGNMENT", "CONDITION"),
		c("MOUTH_ABNORMALITY", "Mouth abnormalities", "Anomalias da boca", r(H, H, H, H, M, L, 10), "DESCRIPTION"),
	}},
	{"CHIN", "Chin/jawline", "Queixo/mandíbula", []Characteristic{
		c("CHIN", "Chin (profile and front)", "Queixo (perfil e frente)", r(M, H, M, L, M, L, 11), "SHAPE", "LENGTH", "WIDTH", "PROMINENCE", "SYMMETRY", "DETAIL"),
		c("JAWLINE", "Jawline (chin to gonial angle)", "Linha da mandíbula (queixo ao ângulo)", r(H, H, M, L, M, L, 11), "SHAPE", "DEFINITION"),
		c("GONIAL_ANGLE", "Gonial angle", "Ângulo da mandíbula", r(H, H, M, L, M, L, 11), "SHAPE", "DEFINITION"),
	}},
	{"NECK", "Neck", "Pescoço", []Characteristic{
		c("NECK", "Neck (overall)", "Pescoço (conjunto)", r(M, H, L, L, L, L, 11), "WIDTH", "HEIGHT", "DETAIL"),
		c("LARYNGEAL", "Laryngeal prominence", "Proeminência laríngea", r(M, H, H, L, M, L, 11), "SHAPE", "SIZE", "PROMINENCE", "LOCATION"),
	}},
	{"FACIAL_HAIR", "Facial hair", "Pelos faciais", []Characteristic{
		c("HAIR_UPPER_LIP", "Facial hair above upper lip", "Pelos acima do lábio superior", r(L, H, L, M, L, L, 12), facialHair...),
		c("HAIR_LOWER_LIP", "Facial hair below lower lip", "Pelos abaixo do lábio inferior", r(L, H, L, M, L, L, 12), facialHair...),
		sided(c("HAIR_SIDE", "Facial hair on the side", "Pelos na lateral da face", r(M, H, L, M, L, L, 12), facialHair...)),
		c("HAIR_NECK", "Facial hair on neck, below chin/jawline", "Pelos no pescoço, abaixo do queixo/mandíbula", r(H, H, L, M, L, L, 12), facialHair...),
	}},
	{"FACIAL_LINES", "Facial lines", "Linhas faciais", []Characteristic{
		c("FRONTAL_LINES", "Frontal lines (forehead)", "Linhas frontais (testa)", r(L, H, M, M, L, L, 12), lines...),
		c("GLABELLAR", "Vertical glabellar line(s)", "Linha(s) glabelar(es) vertical(is)", r(L, M, M, M, L, L, 12), lines...),
		c("NASION_CREASE", "Nasion crease", "Sulco do násio", r(L, H, M, M, L, L, 12), lines...),
		sided(c("LATERAL_NASAL", "Lateral nasal lines", "Linhas nasais laterais", r(L, H, M, H, M, L, 12), lines...)),
		c("BIFID_NOSE", "Bifid nose crease", "Sulco de nariz bífido", r(H, H, H, M, M, L, 12), "DEPTH", "LENGTH"),
		sided(c("PERIORBITAL", "Periorbital lines (crow's feet)", "Linhas periorbitais (pés de galinha)", r(L, M, M, M, L, L, 12), lines...)),
		sided(c("SUPERIOR_PALPEBRAL", "Superior palpebral crease", "Sulco palpebral superior", r(L, H, M, M, L, L, 12), "VISIBILITY", "POSITION", "DEPTH", "SHAPE")),
		sided(c("INFERIOR_PALPEBRAL", "Inferior palpebral crease", "Sulco palpebral inferior", r(L, H, M, M, L, L, 13), "VISIBILITY", "POSITION", "DEPTH", "SHAPE")),
		sided(c("INFRAORBITAL", "Infraorbital creases", "Sulcos infraorbitais", r(L, H, M, M, L, L, 13), lines...)),
		c("UPPER_STRIAE", "Upper circumoral striae", "Estrias periorais superiores", r(L, H, M, M, L, L, 13), lines...),
		c("LOWER_STRIAE", "Lower circumoral striae", "Estrias periorais inferiores", r(L, H, M, M, L, L, 13), lines...),
		c("MENTOLABIAL", "Mentolabial sulcus", "Sulco mentolabial", r(L, H, M, M, L, L, 13), "SHAPE", "LENGTH", "DEPTH"),
		sided(c("NASOLABIAL", "Nasolabial crease/fold", "Sulco nasolabial", r(L, H, M, M, L, M, 13), lines...)),
		sided(c("MARIONETTE", "Marionette lines", "Linhas de marionete", r(L, H, M, M, L, L, 13), "PATTERN", "DEPTH")),
		c("CLEFT_CHIN", "Cleft chin", "Queixo partido", r(H, H, H, M, M, L, 13), "DEPTH", "SIZE"),
		sided(c("BUCCAL", "Buccal creases/folds", "Sulcos bucais", r(L, H, M, M, L, L, 13), lines...)),
		c("NECK_WRINKLES", "Wrinkles on neck", "Rugas do pescoço", r(M, H, L, M, L, L, 13), lines...),
		c("OTHER_CREASES", "Other creases", "Outros sulcos", nil, lines...),
	}},
	{"SCARS", "Scars", "Cicatrizes", []Characteristic{
		c("SCAR", "Scar", "Cicatriz", r(M, M, M, H, H, L, 13), "LOCATION", "SHAPE", "ORIENTATION", "SIZE", "COLOUR", "DEPTH"),
	}},
	{"MARKS", "Facial marks", "Marcas faciais", []Characteristic{
		c("SKIN_MARK", "Skin mark", "Marca na pele", r(M, L, L, M, L, L, 14), "LOCATION", "SHAPE", "SIZE", "COLOUR", "PROMINENCE"),
	}},
	{"ALTERATIONS", "Alterations", "Alterações", []Characteristic{
		c("PIERCING", "Piercing", "Piercing", r(H, M, L, M, M, L, 14), "LOCATION", "DESCRIPTION"),
		c("MAKEUP", "Makeup", "Maquiagem", nil, "LOCATION", "DESCRIPTION"),
		c("TATTOO", "Tattoo (including cosmetic)", "Tatuagem (inclusive cosmética)", r(M, M, M, M, M, L, 14), "LOCATION", "DESCRIPTION"),
		c("OTHER_ALTERATION", "Other alteration", "Outra alteração", nil, "LOCATION", "DESCRIPTION"),
	}},
	{"OTHER", "Other", "Outros", []Characteristic{
		c("OTHER", "Other feature", "Outra característica", nil, "DESCRIPTION", "DETAIL"),
	}},
}

var facialHair = []string{"DISTRIBUTION", "TEXTURE", "SYMMETRY", "DENSITY", "TONE", "ORIENTATION", "EDGE", "CONTINUITY", "LONGER_HAIRS"}

var lines = []string{"DISTRIBUTION", "ORIENTATION", "QUANTITY", "PATTERN", "DEPTH"}

func init() {
	Descriptors["PRESENCE"] = Label{"PRESENCE", "Presence", "Presença"}
}

// Find returns the component and characteristic a code names.
func Find(characteristic string) (Component, Characteristic, bool) {
	for _, comp := range Components {
		for _, ch := range comp.Characteristics {
			if ch.Code == characteristic {
				return comp, ch, true
			}
		}
	}
	return Component{}, Characteristic{}, false
}

// Weakest is the lowest stability of ch under the factors that apply to the pair; "" when ch has no
// rating or no factor applies.
func Weakest(ch Characteristic, factors []string) Stability {
	if ch.Stability == nil {
		return ""
	}
	var worst Stability
	rank := map[Stability]int{High: 3, Medium: 2, Low: 1}
	for _, f := range factors {
		s := ch.Stability.Under(f)
		if s != "" && (worst == "" || rank[s] < rank[worst]) {
			worst = s
		}
	}
	return worst
}
