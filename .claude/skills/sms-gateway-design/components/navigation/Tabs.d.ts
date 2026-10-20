export interface TabItem { id: string; label: React.ReactNode; icon?: string; count?: React.ReactNode; disabled?: boolean }
export interface TabsProps {
  tabs: TabItem[];
  activeId?: string;
  onChange?: (id: string) => void;
  className?: string;
}
export function Tabs(props: TabsProps): JSX.Element;
