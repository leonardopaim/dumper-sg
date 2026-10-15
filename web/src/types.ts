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
  table_presets?: TablePreset[];
}
export type ProfileInput = Omit<Profile, "id" | "has_password"> & {
  password?: string;
};
export type JobKind =
  | "backup"
  | "backup_restore"
  | "restore"
  | "connection_test"
  | "create_database"
  | "table_list"
  | "database_list";
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
  warning_count?: number;
  warning_message?: string;
  partial_result?: boolean;
  cleanup_required?: boolean;
  progress: number;
  message: string;
  exit_code?: number;
  phase?: "backup" | "preparing_restore" | "restore";
  backup_completed?: boolean;
  target_profile_name?: string;
  target_database?: string;
  target_host?: string;
  target_port?: number;
}
export interface JobEvent {
  sequence: number;
  time: string;
  level: string;
  message: string;
}
export interface BackupEntry {
  id?: string;
  path: string;
  name: string;
  modified_at: string;
  size_bytes?: number;
  file_count?: number;
  complete?: boolean;
  problem?: string;
}
export interface Diagnostics {
  available: boolean;
  version: string;
  message: string;
}
export interface ApplicationStatus {
  instance_id: string;
  restart_available: boolean;
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
  tables?: string[] | null;
  restore?: RestoreDestination;
}
export interface RestoreDestination {
  profile_id: number;
  target_database: string;
  threads: number;
  overwrite_tables: boolean;
}
export interface EnvironmentReport {
  checked_at: string;
  platform: string;
  architecture: string;
  go_version: string;
  api_address: string;
  database_path: string;
  directories: {
    name: string;
    path: string;
    exists: boolean;
    readable: boolean;
    writable: boolean;
    message?: string;
  }[];
  docker: {
    available: boolean;
    runtime: string;
    distribution?: string;
    discovery_pending: boolean;
    engine?: string;
    version?: string;
    daemon_id?: string;
    daemon_name?: string;
    operating_system?: string;
    container_os?: string;
    network: string;
    backup_mount?: string;
    backup_mount_accessible: boolean;
    message: string;
    images: { name: string; present: boolean; id?: string; message: string }[];
  };
}
export interface RestoreInput {
  profile_id: number;
  backup_dir: string;
  target_database: string;
  threads: number;
  overwrite_tables: boolean;
  tables?: TableReference[] | null;
}
export type Settings = Record<string, string>;

export interface TableInfo {
  name: string;
  size_bytes: number;
  rows: number;
  table_type: string;
}
export interface TableQuery {
  profile_id: number;
  database: string;
  ssl: boolean;
}

export interface TablePreset {
  name: string;
  database: string;
  tables: string[];
}
export interface TableReference {
  database: string;
  name: string;
}
export interface BackupTableInfo extends TableInfo {
  database: string;
}

export interface DatabaseInfo {
  name: string;
  group_id?: number;
  group_name?: string;
  companies?: CompanyInfo[];
}
export interface CompanyInfo {
  company_id: number;
  group_id: number;
  legal_name: string;
  trade_name: string;
}
export interface DatabaseCatalog {
  databases: DatabaseInfo[];
  warning?: string;
}
export interface BackupSource {
  profile_id: number;
  database: string;
}
