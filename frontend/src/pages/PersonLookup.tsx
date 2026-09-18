import { useState, type FormEvent } from "react";
import { Link } from "react-router";
import PageMeta from "../components/common/PageMeta";
import PageBreadcrumb from "../components/common/PageBreadCrumb";
import ComponentCard from "../components/common/ComponentCard";
import Button from "../components/ui/button/Button";
import {
  Table,
  TableBody,
  TableCell,
  TableHeader,
  TableRow,
} from "../components/ui/table";
import {
  searchPersonsByName,
  type PersonSearchResult,
} from "../services/persons";

// There is no list-persons endpoint (persons are enrolled through an
// organization's own import command, not created here) -- this page finds
// a person by name (see person.SearchByName), for an analyst to pick the
// right match before opening /persons/:personId.
export default function PersonLookup() {
  const [name, setName] = useState("");
  const [searchedFor, setSearchedFor] = useState<string | null>(null);
  const [results, setResults] = useState<PersonSearchResult[]>([]);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState("");

  async function handleSubmit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const trimmed = name.trim();
    if (!trimmed) return;

    setLoading(true);
    setError("");
    try {
      setResults(await searchPersonsByName(trimmed));
      setSearchedFor(trimmed);
    } catch (err) {
      setError(
        err instanceof Error ? err.message : "Could not search persons",
      );
      setResults([]);
      setSearchedFor(trimmed);
    } finally {
      setLoading(false);
    }
  }

  return (
    <>
      <PageMeta
        title="Persons | TrackID"
        description="Search for a person by name"
      />
      <PageBreadcrumb pageTitle="Persons" />

      <ComponentCard
        title="Person Lookup"
        desc="Search enrolled persons by name to open their forensic intelligence profile -- their enrollment identity, resolved biometric clusters, and linked criminal cases."
      >
        <form
          onSubmit={handleSubmit}
          className="flex flex-col gap-3 sm:flex-row sm:items-center"
        >
          <input
            type="text"
            value={name}
            onChange={(event) => setName(event.target.value)}
            placeholder="Search by name..."
            autoFocus
            className="h-11 w-full rounded-lg border border-gray-200 bg-transparent px-4 py-2.5 text-sm text-gray-800 shadow-theme-xs placeholder:text-gray-400 focus:border-brand-300 focus:outline-hidden focus:ring-3 focus:ring-brand-500/10 dark:border-gray-800 dark:bg-gray-900 dark:text-white/90 dark:placeholder:text-white/30 dark:focus:border-brand-800 sm:w-80"
          />
          <Button size="sm" disabled={!name.trim() || loading}>
            {loading ? "Searching..." : "Search"}
          </Button>
        </form>

        {error && <p className="text-sm text-error-500">{error}</p>}

        {searchedFor !== null && !error && (
          <div className="overflow-hidden rounded-xl border border-gray-200 bg-white dark:border-white/[0.05] dark:bg-white/[0.03]">
            <div className="max-w-full overflow-x-auto">
              <Table>
                <TableHeader className="border-b border-gray-100 dark:border-white/[0.05]">
                  <TableRow>
                    <TableCell
                      isHeader
                      className="px-5 py-3 font-medium text-gray-500 text-start text-theme-xs dark:text-gray-400"
                    >
                      Name
                    </TableCell>
                    <TableCell
                      isHeader
                      className="px-5 py-3 font-medium text-gray-500 text-start text-theme-xs dark:text-gray-400"
                    >
                      Document
                    </TableCell>
                    <TableCell
                      isHeader
                      className="px-5 py-3 font-medium text-gray-500 text-start text-theme-xs dark:text-gray-400"
                    >
                      Register
                    </TableCell>
                  </TableRow>
                </TableHeader>
                <TableBody className="divide-y divide-gray-100 dark:divide-white/[0.05]">
                  {results.map((result, index) => (
                    <TableRow key={`${result.personId}-${index}`}>
                      <TableCell className="px-5 py-4 sm:px-6 text-start">
                        <Link
                          to={`/persons/${encodeURIComponent(result.personId)}`}
                          className="block font-medium text-brand-500 hover:text-brand-600 dark:text-brand-400 dark:hover:text-brand-300"
                        >
                          {result.name}
                        </Link>
                      </TableCell>
                      <TableCell className="px-4 py-3 text-gray-500 text-start text-theme-sm dark:text-gray-400">
                        {result.documentType} {result.documentNumber}
                      </TableCell>
                      <TableCell className="px-4 py-3 text-gray-500 text-start text-theme-sm dark:text-gray-400">
                        {result.registerNumber}
                      </TableCell>
                    </TableRow>
                  ))}

                  {!loading && results.length === 0 && (
                    <TableRow>
                      <TableCell className="px-5 py-8 text-center text-gray-500 dark:text-gray-400">
                        No persons found matching &quot;{searchedFor}&quot;.
                      </TableCell>
                    </TableRow>
                  )}
                </TableBody>
              </Table>
            </div>
          </div>
        )}
      </ComponentCard>
    </>
  );
}
