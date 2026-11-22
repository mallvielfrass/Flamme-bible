import { invoke } from "@tauri-apps/api/core";

export interface UpdateOffer {
  current: string;
  latest: string;
  notes: string;
  available: boolean;
  platform: string;
  platform_label: string;
  size: number;
  sha256: string;
  portable: boolean;
  staged: boolean;
  armed: boolean;
  staged_path: string;
}

export interface UpdateProgress {
  received: number;
  total: number;
  percentage: number;
}

export interface DownloadDone {
  path: string;
  portable: boolean;
}

export const updatesApi = {
  check: () => invoke<UpdateOffer>("check_update"),
  download: () => invoke<DownloadDone>("download_update"),
  install: () => invoke<string>("install_update"),
};
