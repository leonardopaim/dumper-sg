export interface Profile {
  id: number;
  name: string;
  host: string;
  port: number;
  user: string;
  database: string;
  ssl: boolean;
  threads: number;
  has_password: boolean;
}
export type ProfileInput = Omit<Profile, "id" | "has_password"> & {
  password?: string;
};
export type JobKind =
  "backup" | "restore" | "connection_test" | "create_database";
export type JobStatus =
  "running" | "cancel_requested" | "succeeded" | "failed" | "cancelled";
export interface Job {
  id: string;
  kind: JobKind;
  status: JobStatus;
  profile_id: number;
  profile_name: string;
  database: string;
  path: string;
  started_at: string;
  finished_at?: string;
  cleanup_required?: boolean;
  progress: number;
  message: string;
  exit_code?: number;
}
export interface JobEvent {
  sequence: number;
  time: string;
  level: string;
  message: string;
}
export interface BackupEntry {
  path: string;
  name: string;
  modified_at: string;
}
export interface Diagnostics {
  available: boolean;
  version: string;
  message: string;
}
export interface BackupInput {
  profile_id: number;
  database: string;
  destination_dir: string;
  threads: number;
  compress: boolean;
  ssl: boolean;
  non_locking: boolean;
  ignore_regex: string;
}
export interface RestoreInput {
  profile_id: number;
  backup_dir: string;
  target_database: string;
  threads: number;
  overwrite_tables: boolean;
}
export type Settings = Record<string, string>;
