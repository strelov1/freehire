import { startErrorTelemetry } from '../../lib/telemetry';
import { mount } from 'svelte';
import App from './App.svelte';
import './app.css';

startErrorTelemetry('panel');

const target = document.getElementById('app');
if (!target) throw new Error('sidepanel.html is missing its #app element');
mount(App, { target });
