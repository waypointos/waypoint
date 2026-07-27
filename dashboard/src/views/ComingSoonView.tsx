import { Sidebar, SidebarBrand } from '@/ui/nav/Sidebar';
import { TopBar } from '@/ui/nav/TopBar';
import { OperatorClock } from '@/ui/nav/OperatorClock';
import { Panel } from '@/ui/primitives/Panel';
import styles from './ComingSoonView.module.css';

type Props = {
  title: string;
  subtitle: string;
  hint: string;
};

export function ComingSoonView({ title, subtitle, hint }: Props) {
  return (
    <div className={styles.shell}>
      <SidebarBrand />
      <TopBar
        left={
          <div className={styles.title}>
            <h1>// {title}</h1>
            <div className={styles.sub}>{subtitle}</div>
          </div>
        }
        right={<OperatorClock />}
      />
      <Sidebar />
      <main className={styles.body}>
        <section className={styles.content}>
          <Panel title={title}>
            <div className={styles.empty}>
              <span className={styles.eyebrow}>// COMING SOON</span>
              <span className={styles.headline}>Not wired up yet</span>
              <span className={styles.hint}>{hint}</span>
            </div>
          </Panel>
        </section>
      </main>
    </div>
  );
}
