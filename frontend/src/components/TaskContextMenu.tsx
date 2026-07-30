import { Pencil, Trash2 } from "lucide-react";
import { useEffect, useRef, type CSSProperties } from "react";

interface TaskContextMenuProps {
  x: number;
  y: number;
  taskTitle: string;
  onEdit(): void;
  onMoveToTrash(): void;
  onClose(): void;
}

const MENU_WIDTH = 196;
const MENU_HEIGHT = 112;
const VIEWPORT_GAP = 8;

export function TaskContextMenu({
  x,
  y,
  taskTitle,
  onEdit,
  onMoveToTrash,
  onClose,
}: TaskContextMenuProps) {
  const menuRef = useRef<HTMLDivElement>(null);
  const style = {
    left: Math.max(
      VIEWPORT_GAP,
      Math.min(x, window.innerWidth - MENU_WIDTH - VIEWPORT_GAP),
    ),
    top: Math.max(
      VIEWPORT_GAP,
      Math.min(y, window.innerHeight - MENU_HEIGHT - VIEWPORT_GAP),
    ),
  } as CSSProperties;

  useEffect(() => {
    const closeOutside = (event: PointerEvent) => {
      if (!menuRef.current?.contains(event.target as Node)) onClose();
    };
    const closeOnEscape = (event: KeyboardEvent) => {
      if (event.key === "Escape") onClose();
    };

    window.addEventListener("pointerdown", closeOutside);
    window.addEventListener("keydown", closeOnEscape);
    window.addEventListener("blur", onClose);
    window.addEventListener("resize", onClose);
    document.addEventListener("scroll", onClose, true);
    return () => {
      window.removeEventListener("pointerdown", closeOutside);
      window.removeEventListener("keydown", closeOnEscape);
      window.removeEventListener("blur", onClose);
      window.removeEventListener("resize", onClose);
      document.removeEventListener("scroll", onClose, true);
    };
  }, [onClose]);

  return (
    <div
      ref={menuRef}
      className="task-context-menu"
      role="menu"
      aria-label={`${taskTitle} 的任务操作`}
      style={style}
    >
      <button type="button" role="menuitem" onClick={onEdit} autoFocus>
        <Pencil size={15} />
        编辑基本信息
      </button>
      <button
        type="button"
        role="menuitem"
        className="danger"
        onClick={onMoveToTrash}
      >
        <Trash2 size={15} />
        移入回收站
      </button>
    </div>
  );
}
