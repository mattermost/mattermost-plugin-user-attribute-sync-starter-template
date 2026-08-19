// Lets TypeScript accept the side-effect `import './foo.scss'` in the components. Webpack handles
// the actual file (see the scss rule in webpack.config.js); without this declaration tsc reports
// the import as an unresolved module.
declare module '*.scss';
