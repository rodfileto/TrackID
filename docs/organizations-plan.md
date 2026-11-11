# Organizations and units: one installation, several police bodies

Decided 2026-10-30, not yet built. One TrackID installation is hosted by one police organization,
but used by others as well, such as PF and PC/MG. Each organization's data is strictly separated
from the others'. Inside an organization, **units** (departments, sections, regional offices)
compartmentalise data access and case distribution.

## Decided 2026-10-30

- **Intelligence has a scope too.** No profile reads across organizations for now. Sharing
  between organizations (agreements, what is shared and with whom) is a later subject.
- **Enrollment registers are configurable:** each one is shared or private to its owner. **Only
  the owner enrolls in it**: for other organizations a shared register is read-only, searched
  but never written.
- **Match thresholds and the identification level stay global**, set for the whole
  installation. Calibration will be global too.
- **Occurrences are recorded per organization.** Several organizations may register the same
  event, each in its own record.
- **Later:** sealed cases (sigilo), and lending examiners between organizations or units.

## Decided 2026-11-11

- **Where a user works is dated.** A user has one **posting** (lotação) at a time and may be sent on
  **mission** (missão) to another unit for a while (`unit_assignments`, migration 029). Both have a
  start and an inclusive end, open while it lasts. A new posting after an open one is a transfer: the
  open one ends the day before. Two postings, or two missions, of one user never overlap. History is
  kept; a record is deleted only when it was entered by mistake.
- **The laudo's header is the examiner's unit's, not the case's.** On the laudo's date the examiner
  works in their mission's unit, else their posting's (`organizations.UnitOn`). That unit is in the
  report data (`reports.Data.Unit`), so it is frozen with the rest when the laudo is issued: a later
  transfer or a finished mission never changes an issued laudo. This replaces decision 7's "from the
  organization and the case's unit". It applies to every laudo, facial and fingerprint, core's
  and a deployment's.
- **What a laudo says of a unit is set on the unit.** `units.laudo_header` holds the header's lines;
  a unit with none takes its nearest ancestor's, and with none in its line the organization's name.
  `units.laudo_name` is how a preâmbulo names the unit, the unit's name when empty. An examiner with
  no unit keeps the installation's header (`reports.Institution`), so a single-organization
  installation changes nothing until units are set.

## What exists

- **No organization or unit anywhere.** One installation is one institute.
  - `users.staff_number`, `username` and `email` are globally unique.
  - Case numbers (`biometric_cases (case_year, case_number)`) and report numbers
    (`analysis_reports (report_type, report_year, report_number)`) are one sequence each.
  - The laudo's header is a constant (`reports.Institution`).
- **Case access is decided in one place**, `distribution.Rights`, and is all-or-nothing by flag:
  - a user who holds an analysis reads and writes that case;
  - an intelligence analyst or a coordinator reads every case;
  - blindness overrides everything (`docs/access-profiles-plan.md`).

  `SeesEveryCase` decides the case list the same way.
- **Admins** (`account_role` ADMIN, SUPER_ADMIN) manage every account. They see no content.
- **Data that crosses cases:**
  - persons and their enrollment (`persons`, `identity_documents`, `identity_registers`);
  - the matcher's SYSTEM decisions, which link traces across cases and to enrolled persons;
  - clusters;
  - occurrences.

  Of these, only intelligence sees person histories, clusters and occurrences. A coordinator sees
  the matcher's candidates on a case page.
- `occurrences.agency` is the police agency that wrote the report (BO). It is outside data about
  the event, not an organization using TrackID, and stays as it is.

## Decisions

1. **Two levels: organization, then units.**
   - An **organization** is the legal body that owns cases and issues laudos: PF, PC/MG.
   - A **unit** belongs to one organization and forms a tree inside it. For example: PF → SR/PF/CE →
     DPF/JNE/CE, and PF → SR/PF/MG → NUCRIM/SETEC.
   - A unit may have a state and a municipality. One with neither is where its parent is, as a sector
     is. Municipalities are IBGE's codes, unchecked against `municipalities` as on `locations`, since
     a deployment may load only its own state's.
   - Organizations are separated strictly. Units compartmentalise within one organization,
     following its hierarchy.
2. **Every case belongs to one organization and one unit.**
   - Its requests, analyses, decisions, reports, files and videos follow the case. They need no
     column of their own, since they hang off the case.
   - The case stores only its unit (`biometric_cases.unit_id`); its organization is the unit's.
   - The unit is the one responsible for the case. It is chosen when the request is registered.
     A case created naming no unit goes to the one unit with `receives_new_cases`.
   - **Owning is not asking.** A request may name the unit that asks
     (`case_requests.requesting_unit_id`), of any organization, while the case belongs to another:
     a state's police can own a case or only request it.
   - A case can be **transferred** to another unit of the same organization, with the transfer
     recorded. It never moves to another organization.
3. **Every user belongs to one organization and to one or more units.**
   - A user's rights over cases reach their units and, for a coordinator, the units below them.
   - An organization moves when its people move: a user changing organization is a new account.
     Their old account keeps naming them on what they decided, as deactivation already does.
4. **Access, by profile.** "Within scope" means the case's unit is one of the user's units or
   below one of them.

   | Profile | Cases they read | What they do |
   |---|---|---|
   | Examiner | the cases where they hold an analysis, as now | decide their step |
   | Coordinator | cases of their organization within scope | register requests, distribute and reassign, open comparisons, in that scope only; assign only users of the same organization |
   | Intelligence | cases of their organization within scope (a user of the root unit reaches the whole organization) | read, link occurrences, persons and clusters within scope; writes only where they hold an analysis, as now |
   | Organization admin | none | manage the accounts and units of their organization |
   | Platform admin (SUPER_ADMIN) | none | manage organizations and the organization admins |

   Blindness still overrides every profile.
5. **Nothing crosses organizations, the matcher included.**
   - The matcher compares a case's traces only with traces of the same organization and with
     enrolled persons in registers that organization reaches: its own, and the shared ones
     (decision 6). No cross-organization link is stored. A cluster that joined two
     organizations' traces would show one organization the other's case through a person's
     history.
   - Inside an organization, units filter what each user sees: a case page or a coordinator sees
     the candidates whose other side is within the user's scope or in a reachable register.
     Intelligence sees persons, clusters and occurrences only as far as its scope.
   - When sharing between organizations is designed, it will open these boundaries explicitly.
     It will not be done by loosening filters.
6. **Enrollment registers have an owner and a sharing setting.**
   - A register (civil identification, criminal identification) belongs to an organization.
   - It is either **shared**, so every organization's cases are matched against it and see its
     candidates, or **private** to its owner.
   - The person's identity section, which examiners see today, is shown when the register is
     within reach.
7. **Numbering and identity are per organization.**
   - Case numbers and laudo numbers run per organization. A case id carries the organization's
     acronym, for example `PCMG-2026.0001`.
   - Staff numbers are unique per organization. Sign-in then needs the organization, chosen on the
     sign-in page or taken from the address.
   - Usernames stay globally unique: they name the decider on every decision.
   - The laudo's header (institution, unit, letterhead) comes from the examiner's unit on the
     laudo's date, not from a constant (decided 2026-11-11, above).
8. **Enforcement has one source and an exhaustive test.**
   - A single scope function, `access.Scope(user)`, returns the organizations and units a user
     reaches.
   - `distribution.Rights`, `SeesEveryCase`, the case list, the request list, distribution, the
     candidates and the reports all go through it.
   - The access-table test (`api/access_db_test.go`) gains two organizations, with nested units
     and a user of each profile in each, and walks every route.
   - **PostgreSQL row-level security is kept for later hardening**, not v1. It would make a
     forgotten filter fail closed, but every request would have to set the user's scope on its
     connection, and the worker and the sim would need their own policies.
9. **Occurrences belong to an organization.**
   - `occurrences` gets its organization.
   - The keys become per organization: TrackID's own number (`occurrence_year`,
     `occurrence_seq`), the police report key (`agency`, `occurrence_number`) and the inquiry
     key. Two organizations can then each record the same BO.
   - Registering a request finds or creates the occurrence within the coordinator's
     organization only.
10. **Audit names organizations.**
   - `access_log` records the reader's organization and the case's organization.
   - Once sharing exists, a cross-organization read will be visible as such.
11. **The UI:**
    - **Organizations** (platform admin): name, acronym, type (federal, state), state, and active
      or not.
    - **Units** (organization admin): the tree, its names, the laudo's header and preâmbulo name,
      the unit receiving new cases, and active or not.
    - **Users:** each user's posting and missions, dated.
    - **Registering a request:** the responsible unit, among the coordinator's units.
    - **Case lists and pages:** the unit, shown to whoever sees several.
    - **Distribution:** only examiners of the case's organization and unit scope.
12. **Migration.**
    - One organization is created from the current installation, with one root unit.
    - Every user, case and register goes into them.
    - Today's admins become that organization's admins. The first SUPER_ADMIN stays the platform
      admin.
    - Nothing changes for a single-organization installation.

## Phases

1. **Organizations, units and membership.**
   - Done (`018_organizations.sql`): the tables, each case's unit, the receiving unit, and the
     requesting unit.
   - Done (`029_unit_assignments.sql`, `organizations`): postings and missions, the Organizations
     and units page (`/admin/organizations`; every admin manages units, a SUPER_ADMIN
     organizations), each user's unit on the Users page, and the laudo's header from the examiner's
     unit. Units in use are deactivated, not deleted.
   - To do: requests choosing the unit.
2. **Scope enforcement.** `access.Scope`, `Rights`, the lists, distribution, coordinators and
   requests, and the access-table test over two organizations.
3. **The boundary.**
   - Matching limited to the organization and its reachable registers, and existing
     cross-organization SYSTEM decisions removed (none exist before the migration).
   - Candidates, persons and clusters filtered by scope.
   - Registers with owner and sharing.
   - Occurrences per organization.
4. **Numbering and documents.** Case and laudo numbers per organization, staff numbers per
   organization with sign-in by organization, and the laudo's header from the organization and
   unit.
5. **Audit and transfers.** `access_log` with organizations, and case transfer between units.
6. **trackid-sim.** A second organization in the world, if the class needs one (its decided rules
   have to be checked first).

## Open

- **Sharing between organizations** (later): which data, with whom, under which agreement, and how
  it is audited.
- **Sealed cases (sigilo)** and **lending examiners** between organizations or units (later).
