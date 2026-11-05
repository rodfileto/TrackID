// React moves these into the document's <head> and swaps them as pages change.
const PageMeta = ({
  title,
  description,
}: {
  title: string;
  description: string;
}) => (
  <>
    <title>{title}</title>
    <meta name="description" content={description} />
  </>
);

export default PageMeta;
