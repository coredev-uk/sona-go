import { create } from "zustand";

interface LayoutStore {
  /** The sidebar is shown (hidden with Ctrl+B to give the page the width). */
  sidebar: boolean;
  toggleSidebar: () => void;
}

export const useLayout = create<LayoutStore>((set) => ({
  sidebar: true,
  toggleSidebar: () => set((s) => ({ sidebar: !s.sidebar })),
}));
