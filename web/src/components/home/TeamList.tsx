import { useNow } from '../../hooks/useNow';
import { Button } from '../ui/Button';
import { Card } from '../ui/Card';
import { Badge } from '../ui/Badge';
import { StatusBadge } from '../ui/StatusBadge';
import { formatCountdown } from '../../utils/format';
import type { Team } from '../../api/types';

interface TeamListProps {
  teams: Team[];
  teamsLoading: boolean;
  isEnabled: boolean;
  joiningTeamId: string | null;
  isLockPending: boolean;
  myTeamIds: Set<string>;
  onJoin: (teamId: string) => void;
}

export function TeamList({
  teams,
  teamsLoading,
  isEnabled,
  joiningTeamId,
  isLockPending,
  myTeamIds,
  onJoin,
}: TeamListProps) {
  const now = useNow();

  return (
    <section>
      <h2 className="text-base font-medium text-[var(--color-text-primary)] mb-4">
        进行中的拼团
      </h2>

      {teamsLoading ? (
        <div className="flex flex-col gap-3">
          {Array.from({ length: 3 }).map((_, i) => (
            <Card key={i} padding="md" className="flex items-center gap-4">
              <div className="w-12 h-12 rounded-full bg-[var(--color-canvas)] animate-pulse flex-shrink-0" />
              <div className="flex-1 space-y-2">
                <div className="h-4 bg-[var(--color-canvas)] rounded animate-pulse w-24" />
                <div className="h-3 bg-[var(--color-canvas)] rounded animate-pulse w-40" />
              </div>
              <div className="h-8 w-16 bg-[var(--color-canvas)] rounded animate-pulse flex-shrink-0" />
            </Card>
          ))}
        </div>
      ) : teams.length === 0 ? (
        <Card padding="sm" className="text-center py-12">
          <p className="text-sm text-[var(--color-text-muted)]">
            来做第一个开团的人
          </p>
        </Card>
      ) : (
        <div className="flex flex-col gap-3">
          {teams.map((team) => {
            const remainingSeconds = Math.max(
              0,
              Math.floor((new Date(team.valid_end).getTime() - now) / 1000),
            );
            const progress =
              team.target_count > 0
                ? Math.round((team.lock_count / team.target_count) * 100)
                : 0;
            const isJoining = joiningTeamId === team.team_id && isLockPending;
            const isMyTeam = myTeamIds.has(team.team_id);

            return (
              <Card key={team.team_id} padding="md" className="flex items-center gap-4">
                {/* Progress ring */}
                <div className="relative w-12 h-12 flex-shrink-0">
                  <svg className="w-12 h-12 -rotate-90" viewBox="0 0 36 36">
                    <circle
                      cx="18" cy="18" r="15"
                      fill="none"
                      stroke="#EAEAEA"
                      strokeWidth="3"
                    />
                    <circle
                      cx="18" cy="18" r="15"
                      fill="none"
                      stroke="var(--color-accent)"
                      strokeWidth="3"
                      strokeDasharray={`${progress * 0.94} 94`}
                      strokeLinecap="round"
                    />
                  </svg>
                  <span className="absolute inset-0 flex items-center justify-center text-xs font-mono font-medium text-[var(--color-text-primary)]">
                    {team.lock_count}/{team.target_count}
                  </span>
                </div>

                {/* Info */}
                <div className="flex-1 min-w-0">
                  <div className="flex items-center gap-2 mb-0.5">
                    <span className="text-sm font-medium text-[var(--color-text-primary)] truncate font-mono">
                      {team.team_id}
                    </span>
                    <StatusBadge type="team" status={team.status} />
                    {isMyTeam && <Badge variant="info">已加入</Badge>}
                  </div>
                  <div className="flex items-center gap-3 text-xs text-[var(--color-text-muted)]">
                    <span>{team.lock_count}人已参与</span>
                    <span>剩余 {formatCountdown(remainingSeconds)}</span>
                  </div>
                </div>

                {/* Join button — different states per team status */}
                {team.status === 1 ? (
                  <span className="text-xs font-medium text-[var(--color-success)] flex-shrink-0 px-2">已成团</span>
                ) : team.status === 2 ? (
                  <span className="text-xs font-medium text-[var(--color-text-muted)] flex-shrink-0 px-2">已失败</span>
                ) : team.status === 3 ? (
                  <span className="text-xs font-medium text-[var(--color-warning)] flex-shrink-0 px-2">已成团(含退款)</span>
                ) : (
                  <Button
                    variant="secondary"
                    size="sm"
                    className="flex-shrink-0"
                    onClick={() => onJoin(team.team_id)}
                    loading={isJoining}
                    disabled={!isEnabled || team.lock_count >= team.target_count}
                  >
                    {team.lock_count >= team.target_count ? '已满' : '加入'}
                  </Button>
                )}
              </Card>
            );
          })}
        </div>
      )}
    </section>
  );
}
