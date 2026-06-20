import { cn } from '../../utils/cn';

interface Column<T> {
  key: string;
  header: string;
  width?: string;
  align?: 'left' | 'center' | 'right';
  render?: (row: T, index: number) => React.ReactNode;
}

interface TableProps<T> {
  columns: Column<T>[];
  data: T[];
  keyExtractor: (row: T) => string | number;
  onRowClick?: (row: T) => void;
  emptyMessage?: string;
  className?: string;
  loading?: boolean;
  loadingRows?: number;
}

export function Table<T>({
  columns,
  data,
  keyExtractor,
  onRowClick,
  emptyMessage = '暂无数据',
  className,
  loading = false,
  loadingRows = 5,
}: TableProps<T>) {
  return (
    <div
      className={cn(
        'border border-[#EAEAEA] rounded-lg overflow-hidden',
        className,
      )}
    >
      <table className="w-full">
        <thead>
          <tr className="bg-[var(--color-canvas)] border-b border-[#EAEAEA]">
            {columns.map((col) => (
              <th
                key={col.key}
                className={cn(
                  'h-10 px-4 text-xs font-medium text-[var(--color-text-secondary)] tracking-wider uppercase',
                  col.align === 'center' && 'text-center',
                  col.align === 'right' && 'text-right',
                  col.align === 'left' && 'text-left',
                )}
                style={col.width ? { width: col.width } : undefined}
              >
                {col.header}
              </th>
            ))}
          </tr>
        </thead>
        <tbody>
          {loading
            ? Array.from({ length: loadingRows }).map((_, i) => (
                <tr
                  key={`skeleton-${i}`}
                  className="border-b border-[#EAEAEA] last:border-0"
                >
                  {columns.map((col) => (
                    <td key={col.key} className="px-4 py-3">
                      <div className="h-4 bg-[var(--color-canvas)] rounded animate-pulse" />
                    </td>
                  ))}
                </tr>
              ))
            : data.length === 0
              ? (
                <tr>
                  <td
                    colSpan={columns.length}
                    className="px-4 py-12 text-center text-sm text-[var(--color-text-muted)]"
                  >
                    {emptyMessage}
                  </td>
                </tr>
              )
              : data.map((row, idx) => (
                <tr
                  key={keyExtractor(row)}
                  className={cn(
                    'border-b border-[#EAEAEA] last:border-0 transition-colors',
                    onRowClick && 'cursor-pointer hover:bg-[var(--color-canvas)]',
                  )}
                  onClick={() => onRowClick?.(row)}
                >
                  {columns.map((col) => (
                    <td
                      key={col.key}
                      className={cn(
                        'px-4 py-3 text-sm text-[var(--color-text-primary)]',
                        col.align === 'center' && 'text-center',
                        col.align === 'right' && 'text-right',
                      )}
                    >
                      {col.render
                        ? col.render(row, idx)
                        : String((row as Record<string, unknown>)[col.key] ?? '')}
                    </td>
                  ))}
                </tr>
              ))}
        </tbody>
      </table>
    </div>
  );
}
