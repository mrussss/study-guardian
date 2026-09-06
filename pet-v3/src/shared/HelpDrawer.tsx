import { useEffect, useRef, type ReactElement } from "react";
import { ArrowUpRight, BookOpen, CheckCircle2, CircleHelp, Clock3, LockKeyhole, Settings2, ShieldCheck, X } from "lucide-react";

export function HelpDrawer({ open, onClose, onNavigate }: { open: boolean; onClose: () => void; onNavigate?: (route: string) => void }): ReactElement | null {
  const drawerRef = useRef<HTMLElement>(null);
  useEffect(() => {
    if (!open) return;
    drawerRef.current?.querySelector<HTMLElement>("button")?.focus();
    const handleKeyDown = (event: KeyboardEvent): void => {
      if (event.key === "Escape") { event.preventDefault(); onClose(); return; }
      if (event.key !== "Tab" || !drawerRef.current) return;
      const focusable = Array.from(drawerRef.current.querySelectorAll<HTMLElement>("button, a, [tabindex]:not([tabindex='-1'])")).filter(item => !item.hasAttribute("disabled"));
      if (focusable.length === 0) return;
      const first = focusable[0]; const last = focusable[focusable.length - 1];
      if (event.shiftKey && document.activeElement === first) { event.preventDefault(); last.focus(); }
      else if (!event.shiftKey && document.activeElement === last) { event.preventDefault(); first.focus(); }
    };
    window.addEventListener("keydown", handleKeyDown);
    return () => window.removeEventListener("keydown", handleKeyDown);
  }, [open, onClose]);
  if (!open) return null;
  const link = (route: string): void => { onClose(); onNavigate?.(route); };
  return <div className="help-drawer-overlay" role="presentation" onMouseDown={event => { if (event.target === event.currentTarget) onClose(); }}>
    <aside className="help-drawer" role="dialog" aria-modal="true" aria-labelledby="help-drawer-title" ref={drawerRef}>
      <header className="help-drawer-header"><div><span className="help-drawer-eyebrow">StudyGuardian 帮助</span><h2 id="help-drawer-title">把专注交给清楚的下一步</h2></div><button className="icon-button" type="button" aria-label="关闭帮助" onClick={onClose}><X size={17} /></button></header>
      <div className="help-drawer-body">
        <section><h3><CheckCircle2 size={16} />三步开始</h3><ol><li>选择当前专注任务。</li><li>点击“开始学习”，让 Supervisor 记录会话。</li><li>需要离开时先休息，结束后查看今日复盘。</li></ol></section>
        <section><h3><BookOpen size={16} />两个任务概念</h3><p><strong>当前专注任务</strong>是你现在正在做的类别；<strong>今日任务</strong>是今天要完成的具体成果。它们可以关联，但不会因为名字相似就自动完成。</p></section>
        <section><h3><ShieldCheck size={16} />状态怎么来的</h3><p>正在专注、暂时离开、状态暂不可用等行为来自本地 Supervisor 的观察。Screen Sensor 负责屏幕活动，ActivityWatch 负责活动数据，Supervisor 负责汇总和安全边界。</p></section>
        <section><h3><LockKeyhole size={16} />本地与隐私</h3><p>原始屏幕内容不会进入任务或复盘页面。设置、Supervisor token 和 AI secret 保存在本机；AI 建议只发送任务标题、描述和本地任务名。</p></section>
        <section><h3><CircleHelp size={16} />常见问题</h3><p>状态暂不可用通常表示活动数据暂时过期；没有累计专注时间时，请确认处于学习模式且活动有效；提醒会受免打扰时段和冷却时间影响。</p></section>
      </div>
      <footer className="help-drawer-footer"><button type="button" onClick={() => link("missions")}><BookOpen size={15} />打开任务页<ArrowUpRight size={14} /></button><button type="button" onClick={() => link("settings")}><Settings2 size={15} />打开设置</button><button type="button" onClick={() => link("system")}><Clock3 size={15} />打开系统状态</button></footer>
    </aside>
  </div>;
}
