package articles

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/rodfileto/trackid/cases"
	"github.com/rodfileto/trackid/db"
)

// TemplateGenerator names the version of the draft generator, logged with every draft, so a
// draft can be told apart from one a later version would write.
const TemplateGenerator = "person-v2"

// Template is a first draft of a person's article, generated from the records and logged
// (article_templates) but not saved as a revision: the analyst edits it, and the save names it.
type Template struct {
	ID          int64           `json:"id"`
	Generator   string          `json:"generator"`
	Language    string          `json:"language"`
	GeneratedAt time.Time       `json:"generatedAt"`
	Body        json.RawMessage `json:"body"`
}

// templateLanguage picks the draft's language: Portuguese for any "pt" tag, else English.
func templateLanguage(lang string) string {
	if strings.HasPrefix(strings.ToLower(lang), "pt") {
		return "pt-BR"
	}
	return "en"
}

// The draft's fixed wording, per language.
var templateText = map[string]map[string]string{
	"en": {
		"born":                 ", born %s",
		"noCases":              "No biometric link to any case, reviewed or not.",
		"leadTodo":             "[Summary: who this person is and why they matter.]",
		"leadCases":            "Examiners confirmed this person's link to %d case(s); %d more rest on links not reviewed or left uncertain.",
		"leadUnconfirmed":      "No link to a case has been confirmed by examiners; %d case(s) rest on links not reviewed or left uncertain.",
		"clusterReviewed":      "reviewed cluster",
		"clusterAutomatic":     "automatic cluster (matcher links, not reviewed)",
		"certaintyLegend":      "[Certainty: confirmed — examiners confirmed the biometric link; not reviewed — the matcher's link or candidate, which no examiner has confirmed: a lead, not a finding; uncertain — examined, but disputed or inconclusive; excluded — examiners ruled it out.]",
		"tier.confirmed":       "Confirmed by examiners",
		"tier.unreviewed":      "Not reviewed (the matcher's links)",
		"tier.uncertain":       "Uncertain after examination",
		"tier.excluded":        "Excluded by examiners",
		"tierShort.confirmed":  "confirmed",
		"tierShort.unreviewed": "not reviewed",
		"pendingScore":         "Matcher candidate, score %s, awaiting review.",
		"pending":              "Matcher candidate, awaiting review.",
		"viaAutomatic":         "Linked through the automatic cluster ",
		"disputed":             "Examiners disagreed; the inconsistency step has not decided.",
		"inconclusive":         "Examiners found the comparison inconclusive.",
		"excluded":             "Examiners excluded the match.",
		"identification":       "Identification",
		"names":                "Registered names: ",
		"aliasTodo":            " — several names: aliases or registration errors? To be checked.",
		"documents":            "Documents: ",
		"parents":              "Parents: %s and %s",
		"bornOn":               "Born: ",
		"biometrics":           "Biometric record",
		"noClusters":           "No reviewed biometric cluster.",
		"facial":               "facial",
		"fingerprint":          "fingerprint",
		"members1":             "1 member",
		"membersN":             "%d members",
		"alsoGroups":           ". Also groups the enrolment of ",
		"sameTodo":             " — the same person under another identity? To be checked.",
		"cases":                "Cases and occurrences",
		"noCasesTodo":          "[No case linked yet.]",
		"occurrence":           "Occurrence ",
		"inquiry":              "inquiry ",
		"requests":             "Request: ",
		"reports":              "Laudo: ",
		"associates":           "Associates",
		"alsoIn":               " — also linked to ",
		"noAssociates":         "No other person is linked to the same cases, reviewed or not. [Add known associates.]",
		"assessment":           "Assessment",
		"assessmentTodo":       "[The analyst's assessment: patterns, modus operandi, risks, open questions.]",
		"and":                  " and ",
	},
	"pt-BR": {
		"born":                 ", nascido(a) em %s",
		"noCases":              "Nenhuma ligação biométrica a caso algum, revisada ou não.",
		"leadTodo":             "[Resumo: quem é esta pessoa e por que interessa.]",
		"leadCases":            "Peritos confirmaram a ligação desta pessoa a %d caso(s); outros %d dependem de ligações não revisadas ou incertas.",
		"leadUnconfirmed":      "Nenhuma ligação a caso foi confirmada por peritos; %d caso(s) dependem de ligações não revisadas ou incertas.",
		"clusterReviewed":      "cluster revisado",
		"clusterAutomatic":     "cluster automático (ligações do comparador, não revisadas)",
		"certaintyLegend":      "[Grau de certeza: confirmado — peritos confirmaram a ligação biométrica; não revisado — ligação ou candidato do comparador que nenhum perito confirmou: uma pista, não uma constatação; incerto — examinado, mas em divergência ou inconclusivo; excluído — peritos descartaram.]",
		"tier.confirmed":       "Confirmados por peritos",
		"tier.unreviewed":      "Não revisados (ligações do comparador)",
		"tier.uncertain":       "Incertos após exame",
		"tier.excluded":        "Excluídos por peritos",
		"tierShort.confirmed":  "confirmado",
		"tierShort.unreviewed": "não revisado",
		"pendingScore":         "Candidato do comparador, escore %s, aguardando revisão.",
		"pending":              "Candidato do comparador, aguardando revisão.",
		"viaAutomatic":         "Ligado pelo cluster automático ",
		"disputed":             "Os peritos divergiram; a etapa de inconsistência ainda não decidiu.",
		"inconclusive":         "Os peritos consideraram a comparação inconclusiva.",
		"excluded":             "Os peritos excluíram a correspondência.",
		"identification":       "Identificação",
		"names":                "Nomes registrados: ",
		"aliasTodo":            " — mais de um nome: vulgos ou erros de cadastro? A verificar.",
		"documents":            "Documentos: ",
		"parents":              "Filiação: %s e %s",
		"bornOn":               "Nascimento: ",
		"biometrics":           "Registro biométrico",
		"noClusters":           "Nenhum cluster biométrico revisado.",
		"facial":               "facial",
		"fingerprint":          "datiloscópico",
		"members1":             "1 membro",
		"membersN":             "%d membros",
		"alsoGroups":           ". Também reúne o cadastro de ",
		"sameTodo":             " — a mesma pessoa sob outra identidade? A verificar.",
		"cases":                "Casos e ocorrências",
		"noCasesTodo":          "[Nenhum caso ligado ainda.]",
		"occurrence":           "Ocorrência ",
		"inquiry":              "inquérito ",
		"requests":             "Requisição: ",
		"reports":              "Laudo: ",
		"associates":           "Pessoas associadas",
		"alsoIn":               " — também ligada a ",
		"noAssociates":         "Nenhuma outra pessoa está ligada aos mesmos casos, por ligação revisada ou não. [Acrescentar associados conhecidos.]",
		"assessment":           "Análise",
		"assessmentTodo":       "[Avaliação do analista: padrões, modus operandi, riscos, questões em aberto.]",
		"and":                  " e ",
	},
}

// draft builds a document node by node.
type draft struct {
	text     map[string]string
	lang     string
	resolved map[MentionKey]*Target
}

func txt(s string) Node    { return Node{Type: "text", Text: s} }
func bold(s string) Node   { return Node{Type: "text", Text: s, Marks: []Mark{{Type: "bold"}}} }
func todo(s string) Node   { return Node{Type: "text", Text: s, Marks: []Mark{{Type: "italic"}}} }
func para(in ...Node) Node { return Node{Type: "paragraph", Content: compact(in)} }
func heading(s string) Node {
	return Node{Type: "heading", Attrs: map[string]any{"level": 2}, Content: []Node{txt(s)}}
}
func item(in ...Node) Node { return Node{Type: "listItem", Content: []Node{para(in...)}} }
func bullets(items ...Node) Node {
	return Node{Type: "bulletList", Content: items}
}

// compact drops empty text nodes, which the schema refuses.
func compact(in []Node) []Node {
	out := in[:0:0]
	for _, n := range in {
		if n.Type == "text" && n.Text == "" {
			continue
		}
		out = append(out, n)
	}
	return out
}

func (d *draft) t(key string) string { return d.text[key] }

func (d *draft) date(t *time.Time) string {
	if t == nil {
		return ""
	}
	if d.lang == "pt-BR" {
		return t.Format("02/01/2006")
	}
	return t.Format("2006-01-02")
}

func (d *draft) modality(m string) string {
	if m == "FACIAL" {
		return d.t("facial")
	}
	return d.t("fingerprint")
}

// mention is a mention node with the target's current label; a target that does not resolve is
// left out of the draft (nil).
func (d *draft) mention(kind Kind, ref string) *Node {
	target := d.resolved[MentionKey{Kind: kind, Ref: ref}]
	if target == nil {
		return nil
	}
	return &Node{Type: "mention", Attrs: map[string]any{"kind": string(kind), "ref": ref, "label": target.Label}}
}

// joined lists mentions separated by commas and a final "and".
func (d *draft) joined(nodes []*Node) []Node {
	var present []Node
	for _, n := range nodes {
		if n != nil {
			present = append(present, *n)
		}
	}
	var out []Node
	for i, n := range present {
		switch {
		case i == 0:
		case i == len(present)-1:
			out = append(out, txt(d.t("and")))
		default:
			out = append(out, txt(", "))
		}
		out = append(out, n)
	}
	return out
}

// Certainty tiers of a link between the person and a case, most certain first.
const (
	tierConfirmed  = "confirmed"  // examiners confirmed it: in a REVIEWED cluster
	tierUncertain  = "uncertain"  // examined, but disputed or inconclusive
	tierUnreviewed = "unreviewed" // the matcher's: a pending candidate or an AUTOMATIC cluster
	tierExcluded   = "excluded"   // examiners excluded it
)

var tierOrder = []string{tierConfirmed, tierUnreviewed, tierUncertain, tierExcluded}

// caseLink is everything the records say about the person and one case.
type caseLink struct {
	caseID      string
	modality    string
	description string
	reviewed    bool                        // in one of the person's REVIEWED clusters
	automatic   int64                       // the AUTOMATIC cluster that links them, 0 for none
	decisions   []cases.PersonTraceDecision // direct pairs, best score first
	statuses    map[string]bool
}

func (c *caseLink) tier() string {
	switch {
	case c.reviewed || c.statuses[cases.IdentityConfirmed]:
		return tierConfirmed
	case c.statuses[cases.IdentityDisputed] || c.statuses[cases.IdentityInconclusive]:
		return tierUncertain
	case c.statuses[cases.IdentityPending] || c.automatic != 0:
		return tierUnreviewed
	}
	return tierExcluded
}

// PersonTemplate writes a first draft of a person's article from what the records link to them.
// Intelligence works with uncertainty, so the draft does not stop at what examiners confirmed:
// each link carries its certainty, from the REVIEWED layer (confirmed), through what examiners
// left disputed or inconclusive, to the matcher's own links (pending candidates with their
// score, and the AUTOMATIC cluster layer) and what examiners excluded. Sections: a lead, the
// identity, the biometric record (both cluster layers, and other enrolments they group), the
// cases by certainty with each case's occurrence, inquiry, requests and issued laudos, the
// persons linked to the same cases, and the assessment. Every fact is a mention; what only the
// analyst can say is left as a bracketed, italic prompt. lang is "pt-BR" or "en".
func PersonTemplate(ctx context.Context, sqlDB *sql.DB, personID, lang string) (Node, error) {
	pl, err := gatherPersonLinks(ctx, sqlDB, personID)
	if err != nil {
		return Node{}, err
	}
	profile := pl.profile
	q := db.New(sqlDB)
	lang = templateLanguage(lang)
	d := &draft{text: templateText[lang], lang: lang}
	links, caseOrder, byTier := pl.cases, pl.caseOrder, pl.byTier
	autoClusters, autoOthers, reviewedOthers := pl.autoClusters, pl.autoOthers, pl.reviewedOthers
	associates, associateOrder := pl.associates, pl.associateOrder

	// Everything the draft may mention, resolved in one go for current labels.
	var keys []MentionKey
	caseTargets := map[string][]MentionKey{}
	for _, id := range caseOrder {
		targets, err := CaseTargets(ctx, sqlDB, id)
		if err != nil {
			return Node{}, err
		}
		caseTargets[id] = targets
		keys = append(keys, targets...)
		if l := links[id]; l.automatic != 0 {
			keys = append(keys, MentionKey{Kind: KindCluster, Ref: strconv.FormatInt(l.automatic, 10)})
		}
	}
	for _, c := range profile.Clusters {
		keys = append(keys, MentionKey{Kind: KindCluster, Ref: strconv.FormatInt(c.ClusterID, 10)})
		for _, p := range reviewedOthers[c.ClusterID] {
			keys = append(keys, MentionKey{Kind: KindPerson, Ref: p})
		}
	}
	for _, id := range autoClusters {
		keys = append(keys, MentionKey{Kind: KindCluster, Ref: strconv.FormatInt(id, 10)})
		for _, p := range autoOthers[id] {
			keys = append(keys, MentionKey{Kind: KindPerson, Ref: p})
		}
	}
	for _, p := range associateOrder {
		keys = append(keys, MentionKey{Kind: KindPerson, Ref: p})
	}
	resolved, err := lookup(ctx, q, keys)
	if err != nil {
		return Node{}, err
	}
	d.resolved = resolved

	// Identity.
	var names []string
	var born, parent1, parent2 string
	var docs []string
	seenName := map[string]bool{}
	for _, doc := range profile.Identity.Documents {
		docs = append(docs, strings.TrimSpace(doc.DocumentType+" "+doc.DocumentNumber))
		for _, r := range doc.Registers {
			if r.Name != "" && !seenName[r.Name] {
				seenName[r.Name] = true
				names = append(names, r.Name)
			}
			if born == "" && r.BirthDate != "" {
				born = r.BirthDate
			}
			if parent1 == "" && (r.Parent1Name != "" || r.Parent2Name != "") {
				parent1, parent2 = r.Parent1Name, r.Parent2Name
			}
		}
	}
	// Registers keep the birth date as text; an ISO date reads in the draft's language.
	if parsed, err := time.Parse("2006-01-02", born); err == nil {
		born = d.date(&parsed)
	}
	name := personID
	if len(names) > 0 {
		name = names[0]
	}

	var doc []Node
	lead := []Node{bold(name), txt(" (" + personID)}
	if born != "" {
		lead = append(lead, txt(fmt.Sprintf(d.t("born"), born)))
	}
	lead = append(lead, txt("). "))
	confirmedN, openN := len(byTier[tierConfirmed]), len(byTier[tierUnreviewed])+len(byTier[tierUncertain])
	switch {
	case confirmedN == 0 && openN == 0:
		lead = append(lead, txt(d.t("noCases")+" "))
	case confirmedN == 0:
		lead = append(lead, txt(fmt.Sprintf(d.t("leadUnconfirmed"), openN)+" "))
	default:
		lead = append(lead, txt(fmt.Sprintf(d.t("leadCases"), confirmedN, openN)+" "))
	}
	lead = append(lead, todo(d.t("leadTodo")))
	doc = append(doc, para(lead...))

	var identity []Node
	if len(names) > 0 {
		in := []Node{txt(d.t("names") + strings.Join(names, "; "))}
		if len(names) > 1 {
			in = append(in, todo(d.t("aliasTodo")))
		}
		identity = append(identity, item(in...))
	}
	if len(docs) > 0 {
		identity = append(identity, item(txt(d.t("documents")+strings.Join(docs, "; "))))
	}
	if born != "" {
		identity = append(identity, item(txt(d.t("bornOn")+born)))
	}
	if parent1 != "" || parent2 != "" {
		identity = append(identity, item(txt(fmt.Sprintf(d.t("parents"), parent1, parent2))))
	}
	doc = append(doc, heading(d.t("identification")))
	if len(identity) > 0 {
		doc = append(doc, bullets(identity...))
	}

	// Biometric record: the reviewed clusters, then the matcher's that hold more.
	doc = append(doc, heading(d.t("biometrics")))
	var clusterItems []Node
	clusterLine := func(id int64, reviewed bool, others []string) {
		m := d.mention(KindCluster, strconv.FormatInt(id, 10))
		if m == nil {
			return
		}
		target := d.resolved[MentionKey{Kind: KindCluster, Ref: strconv.FormatInt(id, 10)}]
		members := d.t("members1")
		if target.Count != 1 {
			members = fmt.Sprintf(d.t("membersN"), target.Count)
		}
		layer := d.t("clusterReviewed")
		if !reviewed {
			layer = d.t("clusterAutomatic")
		}
		in := []Node{*m, txt(" — " + layer + ", " + d.modality(target.Modality) + ", " + members)}
		if len(others) > 0 {
			var ms []*Node
			for _, p := range others {
				ms = append(ms, d.mention(KindPerson, p))
			}
			in = append(in, txt(d.t("alsoGroups")))
			in = append(in, d.joined(ms)...)
			in = append(in, todo(d.t("sameTodo")))
		} else {
			in = append(in, txt("."))
		}
		clusterItems = append(clusterItems, item(in...))
	}
	for _, c := range profile.Clusters {
		clusterLine(c.ClusterID, true, reviewedOthers[c.ClusterID])
	}
	for _, id := range autoClusters {
		// An AUTOMATIC cluster adds something only when it holds more than the reviewed ones.
		target := d.resolved[MentionKey{Kind: KindCluster, Ref: strconv.FormatInt(id, 10)}]
		if target == nil {
			continue
		}
		reviewedMembers := 0
		for _, c := range profile.Clusters {
			if c.Modality == target.Modality {
				reviewedMembers += c.MemberCount
			}
		}
		if target.Count > reviewedMembers || len(autoOthers[id]) > 0 {
			clusterLine(id, false, autoOthers[id])
		}
	}
	if len(clusterItems) == 0 {
		doc = append(doc, para(todo(d.t("noClusters"))))
	} else {
		doc = append(doc, bullets(clusterItems...))
	}

	// Cases, by certainty.
	doc = append(doc, heading(d.t("cases")))
	if len(caseOrder) == 0 {
		doc = append(doc, para(todo(d.t("noCasesTodo"))))
	} else {
		doc = append(doc, para(todo(d.t("certaintyLegend"))))
		for _, tier := range tierOrder {
			if len(byTier[tier]) == 0 {
				continue
			}
			var items []Node
			for _, l := range byTier[tier] {
				if in := d.caseLine(l, tier, caseTargets[l.caseID]); in != nil {
					items = append(items, item(in...))
				}
			}
			if len(items) > 0 {
				doc = append(doc, subheading(d.t("tier."+tier)), bullets(items...))
			}
		}
	}

	doc = append(doc, heading(d.t("associates")))
	sort.SliceStable(associateOrder, func(i, j int) bool {
		return len(associates[associateOrder[i]]) > len(associates[associateOrder[j]])
	})
	var assoc []Node
	for _, p := range associateOrder {
		m := d.mention(KindPerson, p)
		if m == nil {
			continue
		}
		in := []Node{*m, txt(d.t("alsoIn"))}
		for i, s := range associates[p] {
			cm := d.mention(KindCase, s.caseID)
			if cm == nil {
				continue
			}
			if i > 0 {
				in = append(in, txt(", "))
			}
			status := d.t("tierShort." + tierConfirmed)
			if !s.confirmed {
				status = d.t("tierShort." + tierUnreviewed)
			}
			in = append(in, *cm, txt(" ("+status+")"))
		}
		assoc = append(assoc, item(append(in, txt("."))...))
	}
	if len(assoc) > 0 {
		doc = append(doc, bullets(assoc...))
	} else {
		doc = append(doc, para(todo(d.t("noAssociates"))))
	}

	doc = append(doc, heading(d.t("assessment")), para(todo(d.t("assessmentTodo"))))
	return Node{Type: "doc", Content: doc}, nil
}

func subheading(s string) Node {
	return Node{Type: "heading", Attrs: map[string]any{"level": 3}, Content: []Node{txt(s)}}
}

// caseLine is one case's bullet: the case, how certain its link is and on what, and (unless
// excluded) its occurrence, inquiry, requests and issued laudos.
func (d *draft) caseLine(l *caseLink, tier string, targets []MentionKey) []Node {
	m := d.mention(KindCase, l.caseID)
	if m == nil {
		return nil
	}
	in := []Node{*m}
	if t := d.resolved[MentionKey{Kind: KindCase, Ref: l.caseID}]; t != nil && strings.TrimSpace(t.Description) != "" {
		in = append(in, txt(" — "+strings.TrimSuffix(strings.TrimSpace(t.Description), ".")+"."))
	} else {
		in = append(in, txt("."))
	}

	// On what the link rests, when examiners have not confirmed it.
	var best *cases.PersonTraceDecision
	for i := range l.decisions {
		if l.decisions[i].Status == cases.IdentityPending && l.decisions[i].Score != nil {
			best = &l.decisions[i]
			break
		}
	}
	switch tier {
	case tierUnreviewed:
		if best != nil {
			in = append(in, txt(" "+fmt.Sprintf(d.t("pendingScore"), d.score(*best.Score))))
		} else if l.statuses[cases.IdentityPending] {
			in = append(in, txt(" "+d.t("pending")))
		}
		if l.automatic != 0 {
			if cm := d.mention(KindCluster, strconv.FormatInt(l.automatic, 10)); cm != nil {
				in = append(in, txt(" "+d.t("viaAutomatic")), *cm, txt("."))
			}
		}
	case tierUncertain:
		if l.statuses[cases.IdentityDisputed] {
			in = append(in, txt(" "+d.t("disputed")))
		} else {
			in = append(in, txt(" "+d.t("inconclusive")))
		}
	case tierExcluded:
		in = append(in, txt(" "+d.t("excluded")))
		return in
	}

	byKind := map[Kind][]*Node{}
	var occurredOn *time.Time
	for _, k := range targets {
		if k.Kind == KindCase {
			continue
		}
		byKind[k.Kind] = append(byKind[k.Kind], d.mention(k.Kind, k.Ref))
		if k.Kind == KindOccurrence && d.resolved[k] != nil {
			occurredOn = d.resolved[k].Date
		}
	}
	if occ := d.joined(byKind[KindOccurrence]); len(occ) > 0 {
		in = append(in, txt(" "+d.t("occurrence")))
		in = append(in, occ...)
		if occurredOn != nil {
			in = append(in, txt(" ("+d.date(occurredOn)+")"))
		}
		if ipl := d.joined(byKind[KindInquiry]); len(ipl) > 0 {
			in = append(in, txt(", "+d.t("inquiry")))
			in = append(in, ipl...)
		}
		in = append(in, txt("."))
	}
	if reqs := d.joined(byKind[KindRequest]); len(reqs) > 0 {
		in = append(in, txt(" "+d.t("requests")))
		in = append(in, reqs...)
		in = append(in, txt("."))
	}
	if reps := d.joined(byKind[KindReport]); len(reps) > 0 {
		in = append(in, txt(" "+d.t("reports")))
		in = append(in, reps...)
		in = append(in, txt("."))
	}
	return in
}

// score is a matcher score as the draft prints it.
func (d *draft) score(s float64) string {
	out := strconv.FormatFloat(s, 'f', 2, 64)
	if d.lang == "pt-BR" {
		out = strings.Replace(out, ".", ",", 1)
	}
	return out
}

// GenerateTemplate writes a draft of a person's article from the records (PersonTemplate),
// checks it as any body is checked, and logs it in article_templates with who generated it.
// The draft is not a revision: the analyst edits it and saves with SaveInput.TemplateID.
func GenerateTemplate(ctx context.Context, sqlDB *sql.DB, s Subject, lang string, userID int64) (Template, error) {
	if s.Kind != SubjectPerson {
		return Template{}, fmt.Errorf("articles: no template for subject kind %q", s.Kind)
	}
	node, err := PersonTemplate(ctx, sqlDB, s.Ref, lang)
	if err != nil {
		return Template{}, err
	}
	raw, err := json.Marshal(node)
	if err != nil {
		return Template{}, err
	}
	body, err := ParseBody(raw)
	if err != nil {
		return Template{}, fmt.Errorf("articles: generated draft: %w", err)
	}
	lang = templateLanguage(lang)
	row, err := db.New(sqlDB).InsertArticleTemplate(ctx, db.InsertArticleTemplateParams{
		SubjectKind: string(s.Kind), SubjectRef: s.Ref, Generator: TemplateGenerator, Language: lang,
		Body: body.Canonical, BodySha256: body.SHA256[:], GeneratedBy: userID,
	})
	if err != nil {
		return Template{}, err
	}
	return Template{ID: row.ID, Generator: TemplateGenerator, Language: lang, GeneratedAt: row.GeneratedAt, Body: body.Canonical}, nil
}
