import { createContext, useCallback, useContext, useEffect, useMemo, useState, type ReactNode } from 'react';

export type Locale = 'en' | 'es' | 'fr' | 'de' | 'ja' | 'hi';
export const LOCALES: { value: Locale; label: string }[] = [
  { value: 'en', label: 'EN' },
  { value: 'es', label: 'ES' },
  { value: 'fr', label: 'FR' },
  { value: 'de', label: 'DE' },
  { value: 'ja', label: 'JA' },
  { value: 'hi', label: 'HI' },
];

const STORAGE_KEY = 'bluntcode.lang';

type Dict = Record<string, string>;

const en: Dict = {
  'nav.home': 'Home',
  'nav.workspaces': 'Workspaces',
  'nav.search': 'Search',
  'nav.tools': 'Tools',
  'nav.pentest': 'Pentest',
  'nav.settings': 'Settings',
  'nav.about': 'About',
  'nav.more': 'More',
  'category.lint': 'Lint',
  'category.style': 'Style',
  'category.security': 'Security',
  'category.pentest': 'Pentest',
  'category.secrets': 'Secrets',
  'category.maintainability': 'Maintainability',
  'category.dependencies': 'Dependencies',
  'category.container': 'Container',
  'category.iac': 'IaC',
  'category.license': 'License',
  'nav.dashboard': 'Dashboard',
  'nav.report': 'Report',
  'nav.workspace': 'Workspace',
  'nav.scan': 'Scan',
  'common.loading': 'Loading…',
  'common.closeApp': 'Close app',
  'common.addWorkspace': 'Add workspace',
  'common.add': 'Add',
  'common.themeLight': 'Light',
  'common.themeDark': 'Dark',
  'common.switchToLight': 'Switch to light theme',
  'common.switchToDark': 'Switch to dark theme',
  'common.shortcuts': 'Keyboard shortcuts',
  'common.language': 'Language',
  'dashboard.title': 'Workspaces',
  'dashboard.subtitle': 'Run a local scan, then follow every result in one place.',
  'dashboard.overview': 'Overview',
  'dashboard.recentActivity': 'Recent activity',
  'dashboard.recentProjects': 'Recent projects',
  'dashboard.scanLatest': 'Scan latest workspace',
  'report.title': 'Report',
  'workspace.title': 'Workspace',
  'scan.title': 'Scan',
  'scan.liveUpdates': 'Live updates',
  'settings.title': 'Settings',
  'tools.title': 'Tools',
  'common.noAccount': 'No account. No telemetry.',
};

const es: Dict = {
  'nav.home': 'Inicio',
  'nav.workspaces': 'Espacios',
  'nav.search': 'Buscar',
  'nav.tools': 'Herramientas',
  'nav.settings': 'Ajustes',
  'nav.about': 'Acerca',
  'nav.more': 'Más',
  'nav.dashboard': 'Panel',
  'nav.report': 'Informe',
  'nav.workspace': 'Espacio',
  'nav.scan': 'Análisis',
  'common.loading': 'Cargando…',
  'common.closeApp': 'Cerrar app',
  'common.addWorkspace': 'Añadir espacio',
  'common.add': 'Añadir',
  'common.themeLight': 'Claro',
  'common.themeDark': 'Oscuro',
  'common.switchToLight': 'Cambiar a tema claro',
  'common.switchToDark': 'Cambiar a tema oscuro',
  'common.shortcuts': 'Atajos de teclado',
  'common.language': 'Idioma',
  'dashboard.title': 'Espacios',
  'dashboard.subtitle': 'Ejecuta un análisis local y sigue cada resultado en un solo lugar.',
  'dashboard.overview': 'Resumen',
  'dashboard.recentActivity': 'Actividad reciente',
  'dashboard.recentProjects': 'Proyectos recientes',
  'dashboard.scanLatest': 'Analizar último espacio',
  'report.title': 'Informe',
  'workspace.title': 'Espacio',
  'scan.title': 'Análisis',
  'scan.liveUpdates': 'En vivo',
  'settings.title': 'Ajustes',
  'tools.title': 'Herramientas',
  'common.noAccount': 'Sin cuenta. Sin telemetría.',
};

const fr: Dict = {
  'nav.home': 'Accueil',
  'nav.workspaces': 'Espaces',
  'nav.search': 'Rechercher',
  'nav.tools': 'Outils',
  'nav.settings': 'Paramètres',
  'nav.about': 'À propos',
  'nav.more': 'Plus',
  'nav.dashboard': 'Tableau de bord',
  'nav.report': 'Rapport',
  'nav.workspace': 'Espace',
  'nav.scan': 'Analyse',
  'common.loading': 'Chargement…',
  'common.closeApp': 'Fermer',
  'common.addWorkspace': 'Ajouter un espace',
  'common.add': 'Ajouter',
  'common.themeLight': 'Clair',
  'common.themeDark': 'Sombre',
  'common.switchToLight': 'Passer au thème clair',
  'common.switchToDark': 'Passer au thème sombre',
  'common.shortcuts': 'Raccourcis',
  'common.language': 'Langue',
  'dashboard.title': 'Espaces',
  'dashboard.subtitle': 'Lancez une analyse locale et suivez chaque résultat au même endroit.',
  'dashboard.overview': 'Aperçu',
  'dashboard.recentActivity': 'Activité récente',
  'dashboard.recentProjects': 'Projets récents',
  'dashboard.scanLatest': 'Analyser le dernier espace',
  'report.title': 'Rapport',
  'workspace.title': 'Espace',
  'scan.title': 'Analyse',
  'scan.liveUpdates': 'Direct',
  'settings.title': 'Paramètres',
  'tools.title': 'Outils',
  'common.noAccount': 'Sans compte. Sans télémétrie.',
};

const de: Dict = {
  'nav.home': 'Start',
  'nav.workspaces': 'Workspaces',
  'nav.search': 'Suche',
  'nav.tools': 'Werkzeuge',
  'nav.settings': 'Einstellungen',
  'nav.about': 'Über',
  'nav.more': 'Mehr',
  'nav.dashboard': 'Dashboard',
  'nav.report': 'Bericht',
  'nav.workspace': 'Workspace',
  'nav.scan': 'Scan',
  'common.loading': 'Lädt…',
  'common.closeApp': 'App schließen',
  'common.addWorkspace': 'Workspace hinzufügen',
  'common.add': 'Hinzufügen',
  'common.themeLight': 'Hell',
  'common.themeDark': 'Dunkel',
  'common.switchToLight': 'Zum hellen Thema',
  'common.switchToDark': 'Zum dunklen Thema',
  'common.shortcuts': 'Tastenkürzel',
  'common.language': 'Sprache',
  'dashboard.title': 'Workspaces',
  'dashboard.subtitle': 'Starte einen lokalen Scan und verfolge jedes Ergebnis an einem Ort.',
  'dashboard.overview': 'Übersicht',
  'dashboard.recentActivity': 'Letzte Aktivität',
  'dashboard.recentProjects': 'Letzte Projekte',
  'dashboard.scanLatest': 'Letzten Workspace scannen',
  'report.title': 'Bericht',
  'workspace.title': 'Workspace',
  'scan.title': 'Scan',
  'scan.liveUpdates': 'Live',
  'settings.title': 'Einstellungen',
  'tools.title': 'Werkzeuge',
  'common.noAccount': 'Kein Konto. Keine Telemetrie.',
};

const ja: Dict = {
  'nav.home': 'ホーム',
  'nav.workspaces': 'ワークスペース',
  'nav.search': '検索',
  'nav.tools': 'ツール',
  'nav.settings': '設定',
  'nav.about': '概要',
  'nav.more': 'もっと見る',
  'nav.dashboard': 'ダッシュボード',
  'nav.report': 'レポート',
  'nav.workspace': 'ワークスペース',
  'nav.scan': 'スキャン',
  'common.loading': '読み込み中…',
  'common.closeApp': 'アプリを閉じる',
  'common.addWorkspace': 'ワークスペース追加',
  'common.add': '追加',
  'common.themeLight': 'ライト',
  'common.themeDark': 'ダーク',
  'common.switchToLight': 'ライトテーマに切替',
  'common.switchToDark': 'ダークテーマに切替',
  'common.shortcuts': 'ショートカット',
  'common.language': '言語',
  'dashboard.title': 'ワークスペース',
  'dashboard.subtitle': 'ローカルスキャンを実行し、結果を一箇所で確認。',
  'dashboard.overview': '概要',
  'dashboard.recentActivity': '最近のアクティビティ',
  'dashboard.recentProjects': '最近のプロジェクト',
  'dashboard.scanLatest': '最新のワークスペースをスキャン',
  'report.title': 'レポート',
  'workspace.title': 'ワークスペース',
  'scan.title': 'スキャン',
  'scan.liveUpdates': 'ライブ更新',
  'settings.title': '設定',
  'tools.title': 'ツール',
  'common.noAccount': 'アカウント不要。テレメトリなし。',
};

const hi: Dict = {
  'nav.home': 'होम',
  'nav.workspaces': 'कार्यस्थान',
  'nav.search': 'खोजें',
  'nav.tools': 'उपकरण',
  'nav.settings': 'सेटिंग्स',
  'nav.about': 'जानकारी',
  'nav.more': 'और',
  'nav.dashboard': 'डैशबोर्ड',
  'nav.report': 'रिपोर्ट',
  'nav.workspace': 'कार्यस्थान',
  'nav.scan': 'स्कैन',
  'common.loading': 'लोड हो रहा है…',
  'common.closeApp': 'ऐप बंद करें',
  'common.addWorkspace': 'कार्यस्थान जोड़ें',
  'common.add': 'जोड़ें',
  'common.themeLight': 'लाइट',
  'common.themeDark': 'डार्क',
  'common.switchToLight': 'लाइट थीम पर जाएँ',
  'common.switchToDark': 'डार्क थीम पर जाएँ',
  'common.shortcuts': 'शॉर्टकट',
  'common.language': 'भाषा',
  'dashboard.title': 'कार्यस्थान',
  'dashboard.subtitle': 'लोकल स्कैन चलाएँ और सभी परिणाम एक जगह देखें।',
  'dashboard.overview': 'अवलोकन',
  'dashboard.recentActivity': 'हाल की गतिविधि',
  'dashboard.recentProjects': 'हाल के प्रोजेक्ट',
  'dashboard.scanLatest': 'नवीनतम कार्यस्थान स्कैन करें',
  'report.title': 'रिपोर्ट',
  'workspace.title': 'कार्यस्थान',
  'scan.title': 'स्कैन',
  'scan.liveUpdates': 'लाइव',
  'settings.title': 'सेटिंग्स',
  'tools.title': 'उपकरण',
  'common.noAccount': 'कोई खाता नहीं। कोई टेलीमेट्री नहीं।',
};

const dictionaries: Record<Locale, Dict> = { en, es, fr, de, ja, hi };

function resolveLocale(raw: string | null): Locale {
  if (raw === 'es' || raw === 'fr' || raw === 'de' || raw === 'ja' || raw === 'hi' || raw === 'en') return raw;
  return 'en';
}

type I18nContextValue = {
  locale: Locale;
  setLocale: (l: Locale) => void;
  t: (key: string) => string;
};

const I18nContext = createContext<I18nContextValue>({
  locale: 'en',
  setLocale: () => {},
  t: (k) => en[k] ?? k,
});

export function I18nProvider({ children }: { children: ReactNode }) {
  const [locale, setLocaleState] = useState<Locale>(() => {
    try {
      return resolveLocale(window.localStorage.getItem(STORAGE_KEY));
    } catch { return 'en'; }
  });

  const setLocale = useCallback((next: Locale) => {
    setLocaleState(next);
    try { window.localStorage.setItem(STORAGE_KEY, next); } catch { /* ignore */ }
    document.documentElement.lang = next;
  }, []);

  useEffect(() => { document.documentElement.lang = locale; }, [locale]);

  const t = useCallback((key: string) => {
    const d = dictionaries[locale];
    if (d[key] !== undefined) return d[key];
    if (en[key] !== undefined) return en[key];
    return key;
  }, [locale]);

  const value = useMemo(() => ({ locale, setLocale, t }), [locale, setLocale, t]);

  return <I18nContext.Provider value={value}>{children}</I18nContext.Provider>;
}

export function useT() {
  return useContext(I18nContext);
}

export function useI18n() {
  return useContext(I18nContext);
}
