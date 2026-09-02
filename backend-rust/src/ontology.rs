//! Ontology import (BFO + CCO + tid) and Memgraph taxonomy seed, ported from
//! `backend/app/ontology/{importer,models,validation,persistence}.py`.
//!
//! Structural import only — no OWL reasoning. The DL reasoner
//! (`ontology/reasoner.py`, HermiT via ROBOT) stays an offline Java tool.

use std::collections::{HashMap, HashSet};
use std::path::{Path, PathBuf};

use neo4rs::query;
use oxttl::TurtleParser;
use oxrdf::{NamedOrBlankNode, Term};

use crate::graph::GraphService;

// Namespace IRIs.
const RDF_TYPE: &str = "http://www.w3.org/1999/02/22-rdf-syntax-ns#type";
const RDF_FIRST: &str = "http://www.w3.org/1999/02/22-rdf-syntax-ns#first";
const RDF_REST: &str = "http://www.w3.org/1999/02/22-rdf-syntax-ns#rest";
const RDF_NIL: &str = "http://www.w3.org/1999/02/22-rdf-syntax-ns#nil";
const RDFS_SUBCLASS_OF: &str = "http://www.w3.org/2000/01/rdf-schema#subClassOf";
const RDFS_SUBPROPERTY_OF: &str = "http://www.w3.org/2000/01/rdf-schema#subPropertyOf";
const RDFS_LABEL: &str = "http://www.w3.org/2000/01/rdf-schema#label";
const RDFS_DOMAIN: &str = "http://www.w3.org/2000/01/rdf-schema#domain";
const RDFS_RANGE: &str = "http://www.w3.org/2000/01/rdf-schema#range";
const RDFS_COMMENT: &str = "http://www.w3.org/2000/01/rdf-schema#comment";
const RDFS_CLASS: &str = "http://www.w3.org/2000/01/rdf-schema#Class";
const SKOS_DEFINITION: &str = "http://www.w3.org/2004/02/skos/core#definition";
const OWL_CLASS: &str = "http://www.w3.org/2002/07/owl#Class";
const OWL_OBJECT_PROPERTY: &str = "http://www.w3.org/2002/07/owl#ObjectProperty";
const OWL_TRANSITIVE_PROPERTY: &str = "http://www.w3.org/2002/07/owl#TransitiveProperty";
const OWL_INVERSE_OF: &str = "http://www.w3.org/2002/07/owl#inverseOf";
const OWL_DISJOINT_WITH: &str = "http://www.w3.org/2002/07/owl#disjointWith";
const OWL_UNION_OF: &str = "http://www.w3.org/2002/07/owl#unionOf";
const OWL_INTERSECTION_OF: &str = "http://www.w3.org/2002/07/owl#intersectionOf";
const OWL_SOME_VALUES_FROM: &str = "http://www.w3.org/2002/07/owl#someValuesFrom";
const OWL_ALL_VALUES_FROM: &str = "http://www.w3.org/2002/07/owl#allValuesFrom";
const OWL_ON_CLASS: &str = "http://www.w3.org/2002/07/owl#onClass";

const BFO_PREFIX: &str = "http://purl.obolibrary.org/obo/BFO_";
const CCO_PREFIX: &str = "https://www.commoncoreontologies.org/";

// Stable BFO 2.0 class IRIs (see backend/app/ontology/validation.py).
const BFO_ENTITY: &str = "http://purl.obolibrary.org/obo/BFO_0000001";
const BFO_CONTINUANT: &str = "http://purl.obolibrary.org/obo/BFO_0000002";
const BFO_OCCURRENT: &str = "http://purl.obolibrary.org/obo/BFO_0000003";
const BFO_GDC: &str = "http://purl.obolibrary.org/obo/BFO_0000031";
const BFO_MATERIAL_ENTITY: &str = "http://purl.obolibrary.org/obo/BFO_0000040";

#[derive(Debug, Clone)]
pub struct ClassNode {
    pub iri: String,
    pub label: Option<String>,
    pub definition: Option<String>,
    pub source: String,
    pub parents: Vec<String>,
    pub disjoint_with: Vec<String>,
}

#[derive(Debug, Clone)]
#[allow(dead_code)]
pub struct Relation {
    pub iri: String,
    pub label: Option<String>,
    pub definition: Option<String>,
    pub source: String,
    pub domain: Vec<String>,
    pub range: Vec<String>,
    pub transitive: bool,
    pub subproperty_of: Vec<String>,
    pub inverse_of: Vec<String>,
}

#[derive(Debug, Default)]
pub struct Taxonomy {
    pub nodes: HashMap<String, ClassNode>,
    pub relations: HashMap<String, Relation>,
}

impl Taxonomy {
    pub fn get(&self, iri: &str) -> Option<&ClassNode> {
        self.nodes.get(iri)
    }

    #[allow(dead_code)]
    pub fn get_relation(&self, iri: &str) -> Option<&Relation> {
        self.relations.get(iri)
    }

    pub fn ancestors(&self, iri: &str) -> HashSet<String> {
        let mut seen = HashSet::new();
        let mut stack: Vec<String> = self
            .get(iri)
            .map(|n| n.parents.clone())
            .unwrap_or_default();
        while let Some(parent) = stack.pop() {
            if !seen.insert(parent.clone()) {
                continue;
            }
            if let Some(node) = self.get(&parent) {
                stack.extend(node.parents.iter().cloned());
            }
        }
        seen
    }

    #[allow(dead_code)]
    pub fn is_subclass_of(&self, child: &str, parent: &str) -> bool {
        child == parent || self.ancestors(child).contains(parent)
    }
}

fn source_for(iri: &str) -> &'static str {
    if iri.starts_with(BFO_PREFIX) {
        "bfo"
    } else if iri.starts_with(CCO_PREFIX) {
        "cco"
    } else {
        "other"
    }
}

fn collect_ttl_files(dir: &Path, out: &mut Vec<PathBuf>) {
    let Ok(entries) = std::fs::read_dir(dir) else {
        return;
    };
    for entry in entries.flatten() {
        let path = entry.path();
        if path.is_dir() {
            collect_ttl_files(&path, out);
        } else if path.extension().and_then(|e| e.to_str()) == Some("ttl") {
            out.push(path);
        }
    }
}

/// In-memory triple index: subject key -> list of (predicate, object).
type TripleIndex = HashMap<String, Vec<(String, Term)>>;

fn subject_key(subject: &NamedOrBlankNode) -> String {
    match subject {
        NamedOrBlankNode::NamedNode(n) => n.as_str().to_string(),
        NamedOrBlankNode::BlankNode(b) => format!("_:{}", b.as_str()),
    }
}

fn objects_for<'a>(index: &'a TripleIndex, subject: &str, predicate: &str) -> Vec<&'a Term> {
    index
        .get(subject)
        .map(|list| {
            list.iter()
                .filter(|(p, _)| p == predicate)
                .map(|(_, o)| o)
                .collect()
        })
        .unwrap_or_default()
}

fn has_object(index: &TripleIndex, subject: &str, predicate: &str, object: &str) -> bool {
    objects_for(index, subject, predicate).iter().any(|o| match o {
        Term::NamedNode(n) => n.as_str() == object,
        _ => false,
    })
}

fn preferred_literal(index: &TripleIndex, subject: &str, predicate: &str) -> Option<String> {
    let objects = objects_for(index, subject, predicate);
    for o in &objects {
        if let Term::Literal(lit) = o {
            if lit.language() == Some("en") {
                return Some(lit.value().to_string());
            }
        }
    }
    for o in &objects {
        if let Term::Literal(lit) = o {
            if lit.language().is_none() {
                return Some(lit.value().to_string());
            }
        }
    }
    for o in &objects {
        if let Term::Literal(lit) = o {
            return Some(lit.value().to_string());
        }
    }
    None
}

fn definition_for(index: &TripleIndex, subject: &str) -> Option<String> {
    preferred_literal(index, subject, SKOS_DEFINITION)
        .or_else(|| preferred_literal(index, subject, RDFS_COMMENT))
}

fn named_objects(index: &TripleIndex, subject: &str, predicate: &str) -> Vec<String> {
    objects_for(index, subject, predicate)
        .into_iter()
        .filter_map(|o| match o {
            Term::NamedNode(n) => Some(n.as_str().to_string()),
            _ => None,
        })
        .collect()
}

/// Flatten an OWL class expression into the named classes it includes —
/// mirrors the Python importer's `_named_classes_in`.
fn flatten_expr(index: &TripleIndex, term: &Term, visited: &mut HashSet<String>) -> Vec<String> {
    match term {
        Term::NamedNode(n) => vec![n.as_str().to_string()],
        Term::BlankNode(b) => {
            let key = format!("_:{}", b.as_str());
            if !visited.insert(key.clone()) {
                return Vec::new();
            }
            let mut out = Vec::new();
            for (pred, obj) in index.get(&key).cloned().unwrap_or_default() {
                if pred == OWL_UNION_OF || pred == OWL_INTERSECTION_OF {
                    out.extend(flatten_list(index, &obj, visited));
                } else if pred == OWL_SOME_VALUES_FROM
                    || pred == OWL_ALL_VALUES_FROM
                    || pred == OWL_ON_CLASS
                {
                    out.extend(flatten_expr(index, &obj, visited));
                }
            }
            out
        }
        Term::Literal(_) => Vec::new(),
    }
}

/// Traverse an `rdf:List` (union/intersection members), flattening each.
fn flatten_list(index: &TripleIndex, head: &Term, visited: &mut HashSet<String>) -> Vec<String> {
    let mut out = Vec::new();
    let mut current = head.clone();
    let mut seen = HashSet::new();
    loop {
        let key = match &current {
            Term::BlankNode(b) => format!("_:{}", b.as_str()),
            _ => break,
        };
        if !seen.insert(key.clone()) {
            break;
        }
        let firsts = objects_for(index, &key, RDF_FIRST);
        for first in firsts {
            out.extend(flatten_expr(index, first, visited));
        }
        let rests = objects_for(index, &key, RDF_REST);
        match rests.first() {
            Some(Term::NamedNode(n)) if n.as_str() == RDF_NIL => break,
            Some(next) => current = (*next).clone(),
            None => break,
        }
    }
    out
}

pub fn load_taxonomy(dir: &Path) -> anyhow::Result<Taxonomy> {
    let mut files = Vec::new();
    collect_ttl_files(dir, &mut files);
    files.sort();

    let mut index: TripleIndex = HashMap::new();
    for path in &files {
        let reader = std::io::BufReader::new(std::fs::File::open(path)?);
        let parser = TurtleParser::new();
        for triple in parser.for_reader(reader) {
            let triple = triple.map_err(|e| anyhow::anyhow!("parse {}: {e}", path.display()))?;
            let key = subject_key(&triple.subject);
            let pred = triple.predicate.as_str().to_string();
            index
                .entry(key)
                .or_default()
                .push((pred, triple.object));
        }
    }

    let mut taxonomy = Taxonomy::default();

    for (subject, _) in index.clone() {
        if subject.starts_with("_:") {
            continue;
        }
        let is_class = has_object(&index, &subject, RDF_TYPE, OWL_CLASS)
            || has_object(&index, &subject, RDF_TYPE, RDFS_CLASS);
        let is_object_property = has_object(&index, &subject, RDF_TYPE, OWL_OBJECT_PROPERTY);

        if is_class {
            taxonomy.nodes.insert(
                subject.clone(),
                ClassNode {
                    iri: subject.clone(),
                    label: preferred_literal(&index, &subject, RDFS_LABEL),
                    definition: definition_for(&index, &subject),
                    source: source_for(&subject).to_string(),
                    parents: named_objects(&index, &subject, RDFS_SUBCLASS_OF),
                    disjoint_with: named_objects(&index, &subject, OWL_DISJOINT_WITH),
                },
            );
        } else if is_object_property {
            let domain: Vec<String> = objects_for(&index, &subject, RDFS_DOMAIN)
                .into_iter()
                .flat_map(|o| flatten_expr(&index, o, &mut HashSet::new()))
                .collect();
            let range: Vec<String> = objects_for(&index, &subject, RDFS_RANGE)
                .into_iter()
                .flat_map(|o| flatten_expr(&index, o, &mut HashSet::new()))
                .collect();
            taxonomy.relations.insert(
                subject.clone(),
                Relation {
                    iri: subject.clone(),
                    label: preferred_literal(&index, &subject, RDFS_LABEL),
                    definition: definition_for(&index, &subject),
                    source: source_for(&subject).to_string(),
                    domain,
                    range,
                    transitive: has_object(&index, &subject, RDF_TYPE, OWL_TRANSITIVE_PROPERTY),
                    subproperty_of: named_objects(&index, &subject, RDFS_SUBPROPERTY_OF),
                    inverse_of: named_objects(&index, &subject, OWL_INVERSE_OF),
                },
            );
        }
    }

    Ok(taxonomy)
}

pub fn label(taxonomy: &Taxonomy, iri: &str) -> String {
    taxonomy
        .get(iri)
        .and_then(|n| n.label.clone())
        .unwrap_or_else(|| iri.to_string())
}

/// The conformance check (scope "tid": classes whose source is neither BFO nor
/// CCO) — anchoring, reachability to bfo:Entity, and disjointness partition.
pub fn check_conformance(taxonomy: &Taxonomy) -> (bool, Vec<String>) {
    let mut violations = Vec::new();
    for (iri, node) in &taxonomy.nodes {
        if node.source != "other" {
            continue;
        }
        let name = label(taxonomy, iri);

        if node.parents.is_empty() {
            violations.push(format!("{name}: no parent (must be anchored to a BFO/CCO class)"));
        } else {
            for parent in &node.parents {
                if !taxonomy.nodes.contains_key(parent) {
                    violations.push(format!("{name}: dangling parent {parent}"));
                }
            }
        }

        let ancestors = taxonomy.ancestors(iri);
        if !node.parents.is_empty() && !ancestors.contains(BFO_ENTITY) {
            violations.push(format!("{name}: lineage does not reach bfo:Entity"));
        }

        for ancestor in &ancestors {
            if let Some(ancestor_node) = taxonomy.get(ancestor) {
                for other in &ancestor_node.disjoint_with {
                    if ancestors.contains(other) {
                        violations.push(format!(
                            "{name}: under disjoint classes {} and {}",
                            label(taxonomy, ancestor),
                            label(taxonomy, other)
                        ));
                    }
                }
            }
        }
    }
    (violations.is_empty(), violations)
}

// is_* conveniences (ported; usable by a future instance-layer validator).
#[allow(dead_code)]
pub fn is_occurrent(t: &Taxonomy, iri: &str) -> bool {
    t.is_subclass_of(iri, BFO_OCCURRENT)
}
#[allow(dead_code)]
pub fn is_continuant(t: &Taxonomy, iri: &str) -> bool {
    t.is_subclass_of(iri, BFO_CONTINUANT)
}
#[allow(dead_code)]
pub fn is_material_entity(t: &Taxonomy, iri: &str) -> bool {
    t.is_subclass_of(iri, BFO_MATERIAL_ENTITY)
}
#[allow(dead_code)]
pub fn is_information_content_entity(t: &Taxonomy, iri: &str) -> bool {
    t.is_subclass_of(iri, BFO_GDC)
}

/// Persist the taxonomy to Memgraph — idempotent MERGE, matching the Python
/// `persistence.py` shape (OntologyClass / ObjectProperty nodes and
/// SUBCLASS_OF / DOMAIN / RANGE edges).
pub async fn seed_taxonomy(
    graph: &GraphService,
    taxonomy: &Taxonomy,
) -> anyhow::Result<(usize, usize, usize, usize, usize)> {
    for node in taxonomy.nodes.values() {
        graph
            .run_query(
                query(
                    "MERGE (c:OntologyClass {iri: $iri})
                     SET c.name = $name, c.definition = $definition, c.source = $source",
                )
                .param("iri", node.iri.clone())
                .param("name", node.label.clone())
                .param("definition", node.definition.clone())
                .param("source", node.source.clone()),
            )
            .await?;
    }

    let mut subclass_edges = 0;
    for node in taxonomy.nodes.values() {
        for parent in &node.parents {
            if !taxonomy.nodes.contains_key(parent) {
                continue;
            }
            graph
                .run_query(
                    query(
                        "MATCH (c:OntologyClass {iri: $child})
                         MATCH (p:OntologyClass {iri: $parent})
                         MERGE (c)-[:SUBCLASS_OF]->(p)",
                    )
                    .param("child", node.iri.clone())
                    .param("parent", parent.clone()),
                )
                .await?;
            subclass_edges += 1;
        }
    }

    for relation in taxonomy.relations.values() {
        graph
            .run_query(
                query(
                    "MERGE (r:ObjectProperty {iri: $iri})
                     SET r.name = $name, r.definition = $definition,
                         r.transitive = $transitive, r.source = $source",
                )
                .param("iri", relation.iri.clone())
                .param("name", relation.label.clone())
                .param("definition", relation.definition.clone())
                .param("transitive", relation.transitive)
                .param("source", relation.source.clone()),
            )
            .await?;
    }

    let mut domain_edges = 0;
    for relation in taxonomy.relations.values() {
        for cls in &relation.domain {
            if !taxonomy.nodes.contains_key(cls) {
                continue;
            }
            graph
                .run_query(
                    query(
                        "MATCH (r:ObjectProperty {iri: $relation})
                         MATCH (c:OntologyClass {iri: $class})
                         MERGE (r)-[:DOMAIN]->(c)",
                    )
                    .param("relation", relation.iri.clone())
                    .param("class", cls.clone()),
                )
                .await?;
            domain_edges += 1;
        }
    }

    let mut range_edges = 0;
    for relation in taxonomy.relations.values() {
        for cls in &relation.range {
            if !taxonomy.nodes.contains_key(cls) {
                continue;
            }
            graph
                .run_query(
                    query(
                        "MATCH (r:ObjectProperty {iri: $relation})
                         MATCH (c:OntologyClass {iri: $class})
                         MERGE (r)-[:RANGE]->(c)",
                    )
                    .param("relation", relation.iri.clone())
                    .param("class", cls.clone()),
                )
                .await?;
            range_edges += 1;
        }
    }

    Ok((
        taxonomy.nodes.len(),
        taxonomy.relations.len(),
        subclass_edges,
        domain_edges,
        range_edges,
    ))
}
