package cluster

import (
	"context"
	"database/sql"
	"fmt"
	"slices"
	"strings"
	"unicode"

	"github.com/rodfileto/trackid/db"
	"github.com/rodfileto/trackid/graph"
)

// An identity conflict is the relation between clusters and identities failing to be one to one. A
// cluster is one biometric, so it should be one identity's; an identity is one body, so it should sit
// in one cluster per modality.
//
// An identity is not a person row. Nothing merges people: a person is whatever the importer sends, and a
// document from another state, or of another type, can be a row of its own. So records are one identity
// when they share a person row or a fiscal number, or when they share a name and birth date and a fiscal
// number does not contradict it (identityGroups).
//
// Both are raised on the AUTOMATIC layer. Clusters are identified by holding an enrollment, whether or
// not anyone has reviewed it, so that a conflict surfaces before anyone has looked; what people have
// concluded comes alongside, as Strength and each person's Status. The REVIEWED layer cannot raise the
// second kind: a sample nobody has reviewed sits alone there, and "not confirmed the same" is not
// "different".
//
// Two documents with one biometric and one name is normal and is not a conflict, nor is a spelling
// variant of a name.
type ConflictKind string

const (
	// ConflictManyNames is one biometric under different names: a cluster identified as several.
	ConflictManyNames ConflictKind = "ONE_BIOMETRIC_MANY_NAMES"
	// ConflictManyBiometrics is one identity with different biometrics: its records in several
	// clusters of one modality.
	ConflictManyBiometrics ConflictKind = "ONE_IDENTITY_MANY_BIOMETRICS"
)

// ConflictStrength sums up the evidence.
type ConflictStrength string

const (
	// StrengthReviewed: examiners settled the conflict as real.
	StrengthReviewed ConflictStrength = "REVIEWED"
	// StrengthTwoModalities: face and fingerprint both say so, by the matcher alone.
	StrengthTwoModalities ConflictStrength = "TWO_MODALITIES"
	// StrengthOneModality: one modality says so, by the matcher alone. A false match is quite possible.
	StrengthOneModality ConflictStrength = "ONE_MODALITY"
	// StrengthIndirect: no direct comparison, only a chain of matches through other samples.
	StrengthIndirect ConflictStrength = "INDIRECT"
	// StrengthRejected: examiners settled it against the conflict.
	StrengthRejected ConflictStrength = "HUMAN_NEGATIVE"
)

var strengthRank = map[ConflictStrength]int{StrengthReviewed: 0, StrengthTwoModalities: 1, StrengthOneModality: 2, StrengthIndirect: 3, StrengthRejected: 4}

// ConflictDocument is an identity document on a person's records.
type ConflictDocument struct {
	Type   string `json:"type"`
	Number string `json:"number"`
}

// ConflictPerson is a person a conflict involves. Status is their strongest reviewed link to case
// evidence, nil when people have confirmed none: whether this person is identified in the sense of the
// person page, as against the cluster merely holding their enrollment.
type ConflictPerson struct {
	PersonNumber int64              `json:"personNumber"`
	PersonID     string             `json:"personId"`
	Names        []string           `json:"names"`
	BirthDates   []string           `json:"birthDates"`
	Documents    []ConflictDocument `json:"documents"`
	Status       *Link              `json:"status,omitempty"`
}

// ConflictRecord is one enrollment record in a comparison.
type ConflictRecord struct {
	PersonNumber   int64  `json:"personNumber"`
	Name           string `json:"name"`
	DocumentType   string `json:"documentType"`
	DocumentNumber string `json:"documentNumber"`
}

// NoDecision is the Status of a pair nobody and nothing has decided, said outright: the matcher stores
// what it found alike, so an absence is not a finding either way.
const NoDecision EdgeStatus = "NO_DECISION"

// ConflictComparison is what is known of one pair of records in a conflict. Status and SettledBy are
// the pair's decision chain, Status being NoDecision when it has none. Score is the matcher's, or for a face with no
// decision the similarity measured from the embeddings, as Measured says: a low measured score shows two
// records differ, where no score at all only shows they have no match on record.
type ConflictComparison struct {
	Modality  string         `json:"modality"`
	A         ConflictRecord `json:"a"`
	B         ConflictRecord `json:"b"`
	Status    EdgeStatus     `json:"status"`
	SettledBy Role           `json:"settledBy,omitempty"`
	Score     *float64       `json:"score,omitempty"`
	Threshold *float64       `json:"threshold,omitempty"`
	Measured  bool           `json:"measured,omitempty"`
}

// Conflict is one identity conflict.
type Conflict struct {
	Kind       ConflictKind     `json:"kind"`
	Strength   ConflictStrength `json:"strength"`
	Modalities []string         `json:"modalities"`
	// ClusterIDs are the AUTOMATIC clusters it sits in; ReviewedClusterIDs those of the REVIEWED layer
	// that hold it too.
	ClusterIDs         []int64 `json:"clusterIds"`
	ReviewedClusterIDs []int64 `json:"reviewedClusterIds,omitempty"`
	// IdentityBasis says, for ConflictManyBiometrics, what makes the records one identity: PERSON (one
	// person row), FISCAL (a fiscal number across person rows) or NAME_BIRTH (only name and birth date,
	// so possibly a homonym). Strongest that applies.
	IdentityBasis string `json:"identityBasis,omitempty"`
	// Traces counts the case traces in those clusters: what the conflict puts in doubt.
	Traces      int64                `json:"traces"`
	Persons     []ConflictPerson     `json:"persons"`
	Comparisons []ConflictComparison `json:"comparisons"`
	// Registers compare the people's biographic data side by side. Only a page fills them (AddRegisters):
	// parents' names are for the people looking at the page, and stay away from the AI mode's model.
	Registers []ConflictRegister `json:"registers,omitempty"`
}

// ConflictRegister is one identity register of a person in a conflict.
type ConflictRegister struct {
	PersonNumber   int64  `json:"personNumber"`
	DocumentType   string `json:"documentType"`
	DocumentNumber string `json:"documentNumber"`
	FiscalNumber   string `json:"fiscalNumber,omitempty"`
	RegisterNumber string `json:"registerNumber"`
	Name           string `json:"name"`
	BirthDate      string `json:"birthDate,omitempty"`
	Parent1Name    string `json:"parent1Name"`
	Parent2Name    string `json:"parent2Name"`
}

// AddRegisters fills each conflict's Registers, for a page comparing the records.
func AddRegisters(ctx context.Context, sqlDB *sql.DB, conflicts []Conflict) error {
	numbers := []int64{}
	for _, c := range conflicts {
		for _, p := range c.Persons {
			numbers = appendOnceInt(numbers, p.PersonNumber)
		}
	}
	if len(numbers) == 0 {
		return nil
	}
	rows, err := db.New(sqlDB).ListConflictRecords(ctx, numbers)
	if err != nil {
		return fmt.Errorf("cluster: load conflict registers: %w", err)
	}
	byPerson := map[int64][]ConflictRegister{}
	for _, r := range rows {
		byPerson[r.PersonNumber] = append(byPerson[r.PersonNumber], ConflictRegister{
			PersonNumber: r.PersonNumber, DocumentType: r.DocumentType, DocumentNumber: r.DocumentNumber, FiscalNumber: r.FiscalNumber.String, RegisterNumber: r.RegisterNumber,
			Name: r.Name, BirthDate: r.BirthDate.String, Parent1Name: r.Parent1Name, Parent2Name: r.Parent2Name,
		})
	}
	for i := range conflicts {
		for _, p := range conflicts[i].Persons {
			conflicts[i].Registers = append(conflicts[i].Registers, byPerson[p.PersonNumber]...)
		}
	}
	return nil
}

// maxConflictComparisons bounds what one conflict lists; a chain of lookalikes has many pairs.
const maxConflictComparisons = 20

// ListConflicts returns every identity conflict, strongest first. It computes them on each call, over
// the samples that can be in one, so a page and the AI mode always see the same thing and a decision
// counts at once.
func ListConflicts(ctx context.Context, sqlDB *sql.DB) ([]Conflict, error) {
	if sqlDB == nil {
		return nil, fmt.Errorf("database is not configured")
	}
	q := db.New(sqlDB)
	members, err := q.ListConflictCandidateMembers(ctx)
	if err != nil {
		return nil, fmt.Errorf("cluster: list conflict candidates: %w", err)
	}
	if len(members) == 0 {
		return []Conflict{}, nil
	}
	c := conflictBuilder{records: map[string]db.ListConflictCandidateMembersRow{}, persons: map[int64]*ConflictPerson{}, chains: map[[2]string]*pairChain{}}
	for _, m := range members {
		c.records[m.FeatureID] = m
		p := c.persons[m.PersonNumber]
		if p == nil {
			p = &ConflictPerson{PersonNumber: m.PersonNumber, PersonID: m.PersonID, Names: []string{}, BirthDates: []string{}, Documents: []ConflictDocument{}}
			c.persons[m.PersonNumber] = p
		}
		p.Names = appendOnce(p.Names, m.Name)
		if m.BirthDate.Valid {
			p.BirthDates = appendOnce(p.BirthDates, m.BirthDate.String)
		}
		if doc := (ConflictDocument{Type: m.DocumentType, Number: m.DocumentNumber}); !slices.Contains(p.Documents, doc) {
			p.Documents = append(p.Documents, doc)
		}
	}
	features := make([]string, 0, len(c.records))
	files := make([]int64, 0, len(c.records))
	for id, m := range c.records {
		features = append(features, id)
		if m.Modality == "FACIAL" {
			files = append(files, m.IdentityFileID)
		}
	}
	slices.Sort(features)
	decisions, err := q.ListDecisionsBetweenFeatures(ctx, features)
	if err != nil {
		return nil, fmt.Errorf("cluster: list conflict decisions: %w", err)
	}
	for _, d := range decisions {
		k := pairKey(d.FeatureAID, d.FeatureBID)
		chain := c.chains[k]
		if chain == nil {
			chain = &pairChain{}
			c.chains[k] = chain
		}
		chain.entries = append(chain.entries, ChainEntry{Role: Role(d.Role), Decision: Decision(d.Decision), Analysis: d.CaseAnalysisID.Int64,
			AnalysisType: d.AnalysisType.String, Conclusion: Conclusion(d.Conclusion.String)})
		if d.Role == string(RoleSystem) && d.Confidence.Valid {
			score := d.Confidence.Float64
			chain.score = &score
			if d.Threshold.Valid {
				threshold := d.Threshold.Float64
				chain.threshold = &threshold
			}
		}
	}
	faces, err := q.ListFaceScoresBetweenFiles(ctx, files)
	if err != nil {
		return nil, fmt.Errorf("cluster: measure conflict faces: %w", err)
	}
	c.measured = make(map[[2]string]float64, len(faces))
	for _, f := range faces {
		c.measured[pairKey(graph.KnownFeatureID(f.FileA), graph.KnownFeatureID(f.FileB))] = f.Score
	}

	conflicts := append(c.manyNames(members), c.manyBiometrics(members)...)
	if err := c.fill(ctx, q, conflicts); err != nil {
		return nil, err
	}
	slices.SortStableFunc(conflicts, func(a, b Conflict) int {
		if d := strengthRank[a.Strength] - strengthRank[b.Strength]; d != 0 {
			return d
		}
		if a.Kind != b.Kind {
			return strings.Compare(string(a.Kind), string(b.Kind))
		}
		return int(a.Persons[0].PersonNumber - b.Persons[0].PersonNumber)
	})
	return conflicts, nil
}

// ConflictsOfCluster keeps the conflicts sitting in a cluster, of either layer.
func ConflictsOfCluster(conflicts []Conflict, clusterID int64) []Conflict {
	out := []Conflict{}
	for _, c := range conflicts {
		if slices.Contains(c.ClusterIDs, clusterID) || slices.Contains(c.ReviewedClusterIDs, clusterID) {
			out = append(out, c)
		}
	}
	return out
}

// ConflictsOfPerson keeps the conflicts involving a person.
func ConflictsOfPerson(conflicts []Conflict, personNumber int64) []Conflict {
	out := []Conflict{}
	for _, c := range conflicts {
		if slices.ContainsFunc(c.Persons, func(p ConflictPerson) bool { return p.PersonNumber == personNumber }) {
			out = append(out, c)
		}
	}
	return out
}

type pairChain struct {
	entries   []ChainEntry
	score     *float64
	threshold *float64
}

type conflictBuilder struct {
	records  map[string]db.ListConflictCandidateMembersRow
	persons  map[int64]*ConflictPerson
	chains   map[[2]string]*pairChain
	measured map[[2]string]float64
}

func pairKey(a, b string) [2]string {
	if a > b {
		a, b = b, a
	}
	return [2]string{a, b}
}

func appendOnce(list []string, value string) []string {
	if value == "" || slices.Contains(list, value) {
		return list
	}
	return append(list, value)
}

func appendOnceInt(list []int64, value int64) []int64 {
	if slices.Contains(list, value) {
		return list
	}
	return append(list, value)
}

// settled is who settled a pair: humanSame or humanDifferent when people did, machineSame when only the
// matcher found them alike.
func (c *conflictBuilder) settled(a, b string) (humanSame, humanDifferent, machineSame bool) {
	chain := c.chains[pairKey(a, b)]
	if chain == nil {
		return false, false, false
	}
	_, status, by, ok := DecidingRoundBy(chain.entries)
	if !ok {
		return false, false, false
	}
	human := by != RoleSystem
	return human && status == EdgeStatusConfirmed, human && status == EdgeStatusRejected, !human && status == EdgeStatusConfirmed
}

func (c *conflictBuilder) comparison(a, b db.ListConflictCandidateMembersRow) ConflictComparison {
	out := ConflictComparison{
		Modality: a.Modality,
		A:        ConflictRecord{PersonNumber: a.PersonNumber, Name: a.Name, DocumentType: a.DocumentType, DocumentNumber: a.DocumentNumber},
		B:        ConflictRecord{PersonNumber: b.PersonNumber, Name: b.Name, DocumentType: b.DocumentType, DocumentNumber: b.DocumentNumber},
		Status:   NoDecision,
	}
	k := pairKey(a.FeatureID, b.FeatureID)
	if chain := c.chains[k]; chain != nil {
		if _, status, by, ok := DecidingRoundBy(chain.entries); ok {
			out.Status, out.SettledBy = status, by
		}
		out.Score, out.Threshold = chain.score, chain.threshold
	}
	if out.Score == nil {
		if score, ok := c.measured[k]; ok {
			out.Score, out.Measured = &score, true
		}
	}
	return out
}

// manyNames raises one conflict per set of people: a cluster with more than one name, once spelling
// variants are set aside. Clusters of different modalities holding the same people are one conflict.
func (c *conflictBuilder) manyNames(members []db.ListConflictCandidateMembersRow) []Conflict {
	type clusterInfo struct {
		basis    string
		modality string
		records  []db.ListConflictCandidateMembersRow
	}
	clusters := map[int64]*clusterInfo{}
	ids := []int64{}
	for _, m := range members {
		if clusters[m.ClusterID] == nil {
			clusters[m.ClusterID] = &clusterInfo{basis: m.Basis, modality: m.Modality}
			ids = append(ids, m.ClusterID)
		}
		clusters[m.ClusterID].records = append(clusters[m.ClusterID].records, m)
	}
	slices.Sort(ids)

	// What is known of each pair of people, across modalities.
	type evidence struct {
		reviewed, rejected bool
		modalities         map[string]bool
	}
	pairs := map[[2]int64]*evidence{}
	pairOf := func(a, b int64) [2]int64 {
		if a > b {
			a, b = b, a
		}
		return [2]int64{a, b}
	}
	for _, id := range ids {
		for _, group := range conflictPairs(clusters[id].records) {
			for _, a := range group.a {
				for _, b := range group.b {
					k := pairOf(a.PersonNumber, b.PersonNumber)
					if pairs[k] == nil {
						pairs[k] = &evidence{modalities: map[string]bool{}}
					}
					humanSame, humanDifferent, machineSame := c.settled(a.FeatureID, b.FeatureID)
					pairs[k].reviewed = pairs[k].reviewed || humanSame
					pairs[k].rejected = pairs[k].rejected || humanDifferent
					if machineSame || humanSame {
						pairs[k].modalities[a.Modality] = true
					}
				}
			}
		}
	}

	byPeople := map[string]*Conflict{}
	order := []string{}
	type reviewedCluster struct {
		id     int64
		people []int64
	}
	reviewed := []reviewedCluster{}
	for _, id := range ids {
		info := clusters[id]
		groups := conflictPairs(info.records)
		if len(groups) == 0 {
			continue
		}
		people := []int64{}
		for _, r := range info.records {
			people = appendOnceInt(people, r.PersonNumber)
		}
		slices.Sort(people)
		if info.basis != string(BasisAutomatic) {
			reviewed = append(reviewed, reviewedCluster{id, people})
			continue
		}
		key := fmt.Sprint(people)
		conflict := byPeople[key]
		if conflict == nil {
			conflict = &Conflict{Kind: ConflictManyNames, Strength: StrengthRejected, Modalities: []string{}, ClusterIDs: []int64{}, Comparisons: []ConflictComparison{}}
			for _, number := range people {
				conflict.Persons = append(conflict.Persons, *c.persons[number])
			}
			byPeople[key] = conflict
			order = append(order, key)
		}
		conflict.Modalities = appendOnce(conflict.Modalities, info.modality)
		conflict.ClusterIDs = appendOnceInt(conflict.ClusterIDs, id)
		for _, group := range groups {
			for _, a := range group.a {
				for _, b := range group.b {
					e := pairs[pairOf(a.PersonNumber, b.PersonNumber)]
					strength := StrengthIndirect
					switch {
					case e.reviewed:
						strength = StrengthReviewed
					case e.rejected:
						strength = StrengthRejected
					case len(e.modalities) >= 2:
						strength = StrengthTwoModalities
					case len(e.modalities) == 1:
						strength = StrengthOneModality
					}
					if strengthRank[strength] < strengthRank[conflict.Strength] {
						conflict.Strength = strength
					}
					if _, direct := c.chains[pairKey(a.FeatureID, b.FeatureID)]; direct && len(conflict.Comparisons) < maxConflictComparisons {
						conflict.Comparisons = append(conflict.Comparisons, c.comparison(a, b))
					}
				}
			}
		}
	}
	out := make([]Conflict, 0, len(order))
	for _, key := range order {
		conflict := *byPeople[key]
		for _, r := range reviewed {
			inConflict := true
			for _, number := range r.people {
				inConflict = inConflict && slices.ContainsFunc(conflict.Persons, func(p ConflictPerson) bool { return p.PersonNumber == number })
			}
			if inConflict {
				conflict.ReviewedClusterIDs = appendOnceInt(conflict.ReviewedClusterIDs, r.id)
			}
		}
		out = append(out, conflict)
	}
	return out
}

// namePair is two groups of a cluster's records whose names differ.
type namePair struct {
	a, b []db.ListConflictCandidateMembersRow
}

// conflictPairs groups a cluster's records by name, spelling variants together, and returns each pair
// of groups: none when the cluster holds one name, however many documents carry it.
func conflictPairs(records []db.ListConflictCandidateMembersRow) []namePair {
	groups := [][]db.ListConflictCandidateMembersRow{}
	for _, r := range records {
		placed := false
		for i, g := range groups {
			if slices.ContainsFunc(g, func(o db.ListConflictCandidateMembersRow) bool { return SameName(o.Name, r.Name) }) {
				groups[i] = append(g, r)
				placed = true
				break
			}
		}
		if !placed {
			groups = append(groups, []db.ListConflictCandidateMembersRow{r})
		}
	}
	out := []namePair{}
	for i := range groups {
		for j := i + 1; j < len(groups); j++ {
			out = append(out, namePair{groups[i], groups[j]})
		}
	}
	return out
}

// identityGroups sorts records into identities and says what binds each. Records join when they share a
// person row or a fiscal number. Name and birth date join only what has no fiscal number to say otherwise:
// two records with different fiscal numbers are two people however alike their names, and a record with
// none that could belong to either stays apart from both.
func identityGroups(records []db.ListConflictCandidateMembersRow) (groupOf map[string]int, basis map[int]string) {
	parent := make([]int, len(records))
	for i := range parent {
		parent[i] = i
	}
	var find func(int) int
	find = func(i int) int {
		if parent[i] != i {
			parent[i] = find(parent[i])
		}
		return parent[i]
	}
	union := func(a, b int) { parent[find(a)] = find(b) }

	byPerson := map[int64]int{}
	byFiscal := map[string]int{}
	for i, r := range records {
		if first, ok := byPerson[r.PersonNumber]; ok {
			union(first, i)
		} else {
			byPerson[r.PersonNumber] = i
		}
		if fiscal := fiscalOf(r); fiscal != "" {
			if first, ok := byFiscal[fiscal]; ok {
				union(first, i)
			} else {
				byFiscal[fiscal] = i
			}
		}
	}
	type nameBirth struct{ name, birth string }
	byName := map[nameBirth][]int{}
	for i, r := range records {
		if birth := strings.TrimSpace(r.BirthDate.String); birth != "" {
			key := nameBirth{foldName(r.Name), birth}
			byName[key] = append(byName[key], i)
		}
	}
	joinedByName := map[int]bool{}
	for _, members := range byName {
		fiscals := map[string]int{}
		without := []int{}
		for _, i := range members {
			if fiscal := fiscalOf(records[i]); fiscal != "" {
				fiscals[fiscal] = i
			} else {
				without = append(without, i)
			}
		}
		if len(fiscals) > 1 || len(without) == 0 {
			continue // contradicted by fiscal numbers, or those already say all there is
		}
		anchor := without[0]
		for _, i := range fiscals {
			anchor = i
		}
		for _, i := range without {
			if records[i].PersonNumber != records[anchor].PersonNumber {
				joinedByName[i] = true
			}
			union(anchor, i)
		}
	}

	groupOf = make(map[string]int, len(records))
	persons := map[int]map[int64]bool{}
	byNameOnly := map[int]bool{}
	for i, r := range records {
		g := find(i)
		groupOf[r.FeatureID] = g
		if persons[g] == nil {
			persons[g] = map[int64]bool{}
		}
		persons[g][r.PersonNumber] = true
		byNameOnly[g] = byNameOnly[g] || joinedByName[i]
	}
	basis = make(map[int]string, len(persons))
	for g, people := range persons {
		switch {
		case len(people) == 1:
			basis[g] = "PERSON"
		case byNameOnly[g]:
			basis[g] = "NAME_BIRTH"
		default:
			basis[g] = "FISCAL"
		}
	}
	return groupOf, basis
}

func fiscalOf(r db.ListConflictCandidateMembersRow) string {
	return strings.TrimSpace(r.FiscalNumber.String)
}

// manyBiometrics raises one conflict per identity whose records of a modality sit in different AUTOMATIC
// clusters.
func (c *conflictBuilder) manyBiometrics(members []db.ListConflictCandidateMembersRow) []Conflict {
	automatic := []db.ListConflictCandidateMembersRow{}
	for _, m := range members {
		if m.Basis == string(BasisAutomatic) {
			automatic = append(automatic, m)
		}
	}
	groupOf, basis := identityGroups(automatic)
	type key struct {
		group    int
		modality string
	}
	byClusters := map[key]map[int64][]db.ListConflictCandidateMembersRow{}
	for _, m := range automatic {
		k := key{groupOf[m.FeatureID], m.Modality}
		if byClusters[k] == nil {
			byClusters[k] = map[int64][]db.ListConflictCandidateMembersRow{}
		}
		byClusters[k][m.ClusterID] = append(byClusters[k][m.ClusterID], m)
	}
	byGroup := map[int]*Conflict{}
	reviewed := map[int]bool{}
	rejected := map[int]bool{}
	groups := []int{}
	keys := make([]key, 0, len(byClusters))
	for k := range byClusters {
		keys = append(keys, k)
	}
	slices.SortFunc(keys, func(a, b key) int {
		if a.group != b.group {
			return a.group - b.group
		}
		return strings.Compare(a.modality, b.modality)
	})
	for _, k := range keys {
		clusters := byClusters[k]
		if len(clusters) < 2 {
			continue
		}
		conflict := byGroup[k.group]
		if conflict == nil {
			conflict = &Conflict{Kind: ConflictManyBiometrics, IdentityBasis: basis[k.group], Modalities: []string{}, ClusterIDs: []int64{}, Persons: []ConflictPerson{}, Comparisons: []ConflictComparison{}}
			byGroup[k.group] = conflict
			groups = append(groups, k.group)
		}
		conflict.Modalities = appendOnce(conflict.Modalities, k.modality)
		ids := make([]int64, 0, len(clusters))
		for id := range clusters {
			ids = append(ids, id)
			conflict.ClusterIDs = appendOnceInt(conflict.ClusterIDs, id)
		}
		slices.Sort(ids)
		for i := range ids {
			for j := i + 1; j < len(ids); j++ {
				for _, a := range clusters[ids[i]] {
					for _, b := range clusters[ids[j]] {
						humanSame, humanDifferent, _ := c.settled(a.FeatureID, b.FeatureID)
						reviewed[k.group] = reviewed[k.group] || humanDifferent
						rejected[k.group] = rejected[k.group] || humanSame
						if len(conflict.Comparisons) < maxConflictComparisons {
							conflict.Comparisons = append(conflict.Comparisons, c.comparison(a, b))
						}
					}
				}
			}
		}
	}
	people := map[int]map[int64]bool{}
	for _, m := range automatic {
		g := groupOf[m.FeatureID]
		if people[g] == nil {
			people[g] = map[int64]bool{}
		}
		people[g][m.PersonNumber] = true
	}
	out := make([]Conflict, 0, len(groups))
	for _, g := range groups {
		conflict := *byGroup[g]
		numbers := make([]int64, 0, len(people[g]))
		for number := range people[g] {
			numbers = append(numbers, number)
		}
		slices.Sort(numbers)
		for _, number := range numbers {
			conflict.Persons = append(conflict.Persons, *c.persons[number])
		}
		switch {
		case reviewed[g]:
			conflict.Strength = StrengthReviewed
		case rejected[g]:
			conflict.Strength = StrengthRejected
		case len(conflict.Modalities) >= 2:
			conflict.Strength = StrengthTwoModalities
		default:
			conflict.Strength = StrengthOneModality
		}
		out = append(out, conflict)
	}
	return out
}

// fill adds what the conflicts need from outside the candidates: traces and each person's status.
func (c *conflictBuilder) fill(ctx context.Context, q *db.Queries, conflicts []Conflict) error {
	ids := []int64{}
	for _, conflict := range conflicts {
		ids = append(ids, conflict.ClusterIDs...)
	}
	counts, err := q.ListClusterTraceCounts(ctx, ids)
	if err != nil {
		return fmt.Errorf("cluster: count conflict traces: %w", err)
	}
	traces := make(map[int64]int64, len(counts))
	for _, r := range counts {
		traces[r.ClusterID] = r.Traces
	}
	features := []string{}
	featureOwner := map[string]int64{}
	for id, m := range c.records {
		features = append(features, id)
		featureOwner[id] = m.PersonNumber
	}
	slices.Sort(features)
	level, err := IdentificationLevel(ctx, q)
	if err != nil {
		return err
	}
	rows, err := q.ListBiometricDecisionsForFeatures(ctx, features)
	if err != nil {
		return fmt.Errorf("cluster: load conflict status: %w", err)
	}
	chains := map[[2]string][]ChainEntry{}
	for _, r := range rows {
		if _, isTrace := graph.ParseQuestionedFeatureID(r.CounterpartID); !isTrace {
			continue
		}
		k := [2]string{r.FeatureID, r.CounterpartID}
		chains[k] = append(chains[k], ChainEntry{Role: Role(r.Role), Decision: Decision(r.Decision), Analysis: r.CaseAnalysisID.Int64,
			AnalysisType: r.AnalysisType.String, Conclusion: Conclusion(r.Conclusion.String)})
	}
	status := map[int64]*Link{}
	for k, chain := range chains {
		link, ok := ReviewedLink(chain, level)
		if !ok {
			continue
		}
		owner := featureOwner[k[0]]
		if best := status[owner]; best == nil || LinkStrength(link) > LinkStrength(*best) {
			status[owner] = &link
		}
	}
	for i := range conflicts {
		for _, id := range conflicts[i].ClusterIDs {
			conflicts[i].Traces += traces[id]
		}
		slices.Sort(conflicts[i].ClusterIDs)
		for j := range conflicts[i].Persons {
			conflicts[i].Persons[j].Status = status[conflicts[i].Persons[j].PersonNumber]
		}
	}
	return nil
}

// LinkStrength orders reviewed links: a laudo at the identification level over a lead, and a lead over
// a laudo below it.
func LinkStrength(l Link) int {
	switch {
	case l.Basis == LinkLaudo && l.Identified:
		return 3
	case l.Basis == LinkLead:
		return 2
	}
	return 1
}

// SameName says two names are one name, or a spelling variant of it: the same once case and accents are
// set aside and initials dropped, or a couple of keystrokes apart ("Sillva", "Katai"). Registers have
// typos, and those are not another identity.
func SameName(a, b string) bool {
	a, b = foldName(a), foldName(b)
	if a == b {
		return true
	}
	return editDistance(a, b) <= 2
}

var accents = strings.NewReplacer("á", "a", "à", "a", "â", "a", "ã", "a", "é", "e", "ê", "e", "í", "i", "ó", "o", "ô", "o", "õ", "o", "ú", "u", "ü", "u", "ç", "c")

// foldName is a name in lower case, without accents, punctuation or initials.
func foldName(name string) string {
	folded := accents.Replace(strings.ToLower(name))
	words := []string{}
	for _, word := range strings.FieldsFunc(folded, func(r rune) bool { return !unicode.IsLetter(r) }) {
		if len([]rune(word)) > 1 {
			words = append(words, word)
		}
	}
	return strings.Join(words, " ")
}

// editDistance is the Levenshtein distance.
func editDistance(a, b string) int {
	ra, rb := []rune(a), []rune(b)
	row := make([]int, len(rb)+1)
	for j := range row {
		row[j] = j
	}
	for i := 1; i <= len(ra); i++ {
		previous := row[0]
		row[0] = i
		for j := 1; j <= len(rb); j++ {
			cost := 1
			if ra[i-1] == rb[j-1] {
				cost = 0
			}
			current := row[j]
			row[j] = min(row[j]+1, row[j-1]+1, previous+cost)
			previous = current
		}
	}
	return row[len(rb)]
}
