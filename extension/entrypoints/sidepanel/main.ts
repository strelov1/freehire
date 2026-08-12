import { mount } from 'svelte';
import App from './App.svelte';
import './app.css';

const target = document.getElementById('app');
if (!target) throw new Error('sidepanel.html is missing its #app element');
mount(App, { target });
