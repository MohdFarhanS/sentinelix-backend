-- Bersihkan orphan rows dulu (monitor_checks yang monitor_id-nya sudah
-- tidak ada di monitors — hasil dari bug DELETE tanpa FK sebelum
-- migration ini). Di environment fresh, ini no-op (tidak ada row yang
-- match kondisi NOT EXISTS).
DELETE FROM monitor_checks mc
WHERE NOT EXISTS (
    SELECT 1 FROM monitors m WHERE m.id = mc.monitor_id
);

-- FK constraint yang seharusnya ada dari awal di 000005 — supaya
-- DELETE /monitors/:id beneran cascade ke monitor_checks di level DB,
-- sesuai yang dijanjikan di 04-API-DESIGN.md §7.
ALTER TABLE monitor_checks
    ADD CONSTRAINT fk_monitor_checks_monitor
    FOREIGN KEY (monitor_id) REFERENCES monitors(id) ON DELETE CASCADE;