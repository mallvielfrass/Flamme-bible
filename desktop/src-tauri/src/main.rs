#![cfg_attr(not(debug_assertions), windows_subsystem = "windows")]

fn main() {
    if flammebible_desktop::apply_pending_update() {
        return;
    }
    flammebible_desktop::run();
}
