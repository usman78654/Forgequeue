const destinations = [
  ['Metrics', import.meta.env.VITE_METRICS_URL],
  ['Traces', import.meta.env.VITE_TRACES_URL],
  ['Runbook', import.meta.env.VITE_RUNBOOK_URL],
] as const;

export function ObservabilityLinks({resource}: {resource: string}) {
  return (
    <div className="row-actions">
      {destinations.flatMap(([label, configured]) => {
        if (!configured) return [];
        try {
          const url = new URL(configured);
          if (!['https:', 'http:'].includes(url.protocol)) return [];
          url.searchParams.set('forgequeue_resource', resource);
          return [
            <a key={label} href={url.href} target="_blank" rel="noopener noreferrer">
              {label}
            </a>,
          ];
        } catch {
          return [];
        }
      })}
    </div>
  );
}
