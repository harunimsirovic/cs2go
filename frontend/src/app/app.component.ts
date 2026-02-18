import { CommonModule } from '@angular/common';
import { Component, ElementRef, OnDestroy, ViewChild } from '@angular/core';

interface JobState {
  id: string;
  status: 'pending' | 'processing' | 'done' | 'error';
  progress: number;
}

interface LogEntry {
  message: string;
  type: '' | 'ok' | 'info' | 'err';
}

interface Insight {
  category: string;
  message: string;
  severity: 'tip' | 'warning' | 'critical';
}

interface Player {
  name?: string;
  team?: 'CT' | 'T' | string;
  kills?: number;
  deaths?: number;
  headshot_kills?: number;
  shots_fired?: number;
  shots_hit?: number;
  grenades_thrown?: number;
  flashes_thrown?: number;
  smokes_thrown?: number;
  total_money_spent?: number;
  hit_locations?: Record<string, number>;
  insights?: Insight[];
}

interface RoundInfo {
  round?: number;
  winner?: string;
  reason?: string;
  duration_seconds?: number;
  kill_log?: string[] | string;
}

interface AnalysisResult {
  map_name?: string;
  total_rounds?: number;
  duration_seconds?: number;
  players?: Record<string, Player>;
  round_log?: RoundInfo[];
}

interface UploadResponse {
  job_id: string;
}

interface JobUpdate {
  status: JobState['status'];
  progress: number;
}

@Component({
  selector: 'app-root',
  standalone: true,
  imports: [CommonModule],
  templateUrl: './app.component.html',
  styleUrls: ['./app.component.css']
})
export class AppComponent implements OnDestroy {
  @ViewChild('fileInput') fileInput?: ElementRef<HTMLInputElement>;

  readonly jobs: JobState[] = [];
  readonly logs: LogEntry[] = [{ message: 'system ready. waiting for demo file...', type: 'info' }];
  readonly sockets = new Map<string, WebSocket>();
  isDragOver = false;
  results: AnalysisResult | null = null;

  get hasResults(): boolean {
    return this.results !== null;
  }

  get players(): Player[] {
    return Object.values(this.results?.players ?? {});
  }

  triggerFileDialog(): void {
    this.fileInput?.nativeElement.click();
  }

  onDragOver(event: DragEvent): void {
    event.preventDefault();
    this.isDragOver = true;
  }

  onDragLeave(): void {
    this.isDragOver = false;
  }

  onDrop(event: DragEvent): void {
    event.preventDefault();
    this.isDragOver = false;
    const file = event.dataTransfer?.files?.item(0);
    if (file) {
      void this.uploadFile(file);
    }
  }

  onFileSelected(event: Event): void {
    const input = event.target as HTMLInputElement;
    const file = input.files?.item(0);
    if (file) {
      void this.uploadFile(file);
    }
    input.value = '';
  }

  playerAccuracy(player: Player): string {
    const shotsFired = player.shots_fired ?? 0;
    const shotsHit = player.shots_hit ?? 0;
    if (shotsFired <= 0) {
      return '0.0';
    }
    return ((shotsHit / shotsFired) * 100).toFixed(1);
  }

  headshotRate(player: Player): string {
    const kills = player.kills ?? 0;
    const headshots = player.headshot_kills ?? 0;
    if (kills <= 0) {
      return '0';
    }
    return ((headshots / kills) * 100).toFixed(0);
  }

  kdRatio(player: Player): string {
    const kills = player.kills ?? 0;
    const deaths = player.deaths ?? 0;
    if (deaths <= 0) {
      return kills.toFixed(2);
    }
    return (kills / deaths).toFixed(2);
  }

  utilityUsed(player: Player): number {
    return (player.grenades_thrown ?? 0) + (player.flashes_thrown ?? 0) + (player.smokes_thrown ?? 0);
  }

  formatMoney(amount: number | undefined): string {
    if (amount === undefined) {
      return '0';
    }
    return amount.toLocaleString();
  }

  formatDuration(seconds: number | undefined): string {
    if (seconds === undefined) {
      return '0m 0s';
    }
    const minutes = Math.floor(seconds / 60);
    const remaining = Math.floor(seconds % 60);
    return `${minutes}m ${remaining}s`;
  }

  trackByJobId(_: number, job: JobState): string {
    return job.id;
  }

  trackByPlayer(_: number, player: Player): string {
    return `${player.name ?? 'unknown'}-${player.team ?? '?'}`;
  }

  formatKillLog(killLog: string[] | string | undefined): string {
    if (Array.isArray(killLog)) {
      return killLog.join(', ');
    }
    return killLog || '-';
  }

  topHitLocations(hitLocations?: Record<string, number>): Array<{ name: string; percent: number; isHead: boolean }> {
    const entries = Object.entries(hitLocations ?? {});
    if (!entries.length) {
      return [];
    }

    const total = entries.reduce((sum, [, count]) => sum + count, 0) || 1;
    return entries
      .sort((a, b) => b[1] - a[1])
      .slice(0, 4)
      .map(([name, count]) => {
        const percent = Number(((count / total) * 100).toFixed(0));
        return { name, percent, isHead: name === 'Head' };
      });
  }

  ngOnDestroy(): void {
    this.sockets.forEach((socket) => socket.close());
    this.sockets.clear();
  }

  private async uploadFile(file: File): Promise<void> {
    if (!file.name.endsWith('.dem')) {
      this.log('error: only .dem files accepted', 'err');
      return;
    }

    const sizeMb = (file.size / 1024 / 1024).toFixed(1);
    this.log(`uploading ${file.name} (${sizeMb} MB)...`, 'info');

    const form = new FormData();
    form.append('demo', file);

    try {
      const response = await fetch('/upload', { method: 'POST', body: form });
      const data = (await response.json()) as UploadResponse & { error?: string };

      if (!response.ok) {
        this.log(`upload error: ${data.error ?? response.statusText}`, 'err');
        return;
      }

      this.log(`job created: ${data.job_id}`, 'ok');
      this.addJob(data.job_id);
      this.connectWebSocket(data.job_id);
    } catch (error) {
      this.log(`network error: ${(error as Error).message}`, 'err');
    }
  }

  private addJob(id: string): void {
    this.jobs.unshift({
      id,
      status: 'pending',
      progress: 0
    });
  }

  private updateJob(id: string, status: JobState['status'], progress: number): void {
    const current = this.jobs.find((job) => job.id === id);
    if (!current) {
      return;
    }
    current.status = status;
    current.progress = progress;
  }

  private connectWebSocket(jobId: string): void {
    const protocol = window.location.protocol === 'https:' ? 'wss' : 'ws';
    const socket = new WebSocket(`${protocol}://${window.location.host}/ws?job_id=${jobId}`);

    socket.onopen = () => this.log(`ws connected for ${jobId.slice(0, 8)}...`, 'info');
    socket.onclose = () => this.log(`ws closed for ${jobId.slice(0, 8)}`, '');
    socket.onerror = () => this.log('ws error', 'err');

    socket.onmessage = (event: MessageEvent<string>) => {
      try {
        const message = JSON.parse(event.data) as JobUpdate;
        this.updateJob(jobId, message.status, message.progress);
        this.log(`[${jobId.slice(0, 8)}] ${message.status} ${message.progress}%`);

        if (message.status === 'done') {
          socket.close();
          this.sockets.delete(jobId);
          void this.loadResults(jobId);
        }
      } catch {
        this.log('ws message parse error', 'err');
      }
    };

    this.sockets.set(jobId, socket);
  }

  private async loadResults(jobId: string): Promise<void> {
    this.log(`fetching results for ${jobId.slice(0, 8)}...`, 'info');
    try {
      const response = await fetch(`/jobs/${jobId}/result`);
      const data = (await response.json()) as AnalysisResult & { error?: string };

      if (!response.ok) {
        this.log(`result error: ${data.error ?? 'unknown'}`, 'err');
        return;
      }

      this.results = data;
      this.log('results rendered', 'ok');
      window.requestAnimationFrame(() => {
        document.getElementById('results-section')?.scrollIntoView({ behavior: 'smooth' });
      });
    } catch (error) {
      this.log(`failed to load results: ${(error as Error).message}`, 'err');
    }
  }

  private log(message: string, type: LogEntry['type'] = ''): void {
    this.logs.push({ message, type });
  }
}
