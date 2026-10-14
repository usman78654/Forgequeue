interface PaginationProps {
  pageNumber: number;
  hasPrevious: boolean;
  hasNext: boolean;
  loading: boolean;
  next: () => void;
  previous: () => void;
}

export function Pagination({
  pageNumber,
  hasPrevious,
  hasNext,
  loading,
  next,
  previous,
}: PaginationProps) {
  return (
    <nav className="toolbar" aria-label="Result pages">
      <button onClick={previous} disabled={loading || !hasPrevious}>
        Previous page
      </button>
      <span>Page {pageNumber}</span>
      <button onClick={next} disabled={loading || !hasNext}>
        Next page
      </button>
    </nav>
  );
}
