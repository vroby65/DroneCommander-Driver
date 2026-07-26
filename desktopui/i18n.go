package desktopui

import "strings"

const languagePreference = "interface-language"

type languageOption struct {
	Code string
	Name string
}

type commandEditorMessages struct {
	Title, CommandsTab, XMLTab, Help, Unavailable, Invalid, Sync, TextToXML, XMLToText, Save string
}

// Keep this list aligned with Drone Commander's language selector.
var supportedLanguages = []languageOption{
	{Code: "en", Name: "English"},
	{Code: "it", Name: "Italiano"},
	{Code: "fr", Name: "Français"},
	{Code: "de", Name: "Deutsch"},
	{Code: "es", Name: "Español"},
	{Code: "pt", Name: "Português"},
	{Code: "ar", Name: "العربية"},
	{Code: "zh", Name: "简体中文"},
	{Code: "ko", Name: "한국어"},
	{Code: "ja", Name: "日本語"},
}

var confirmationLabels = map[string][2]string{
	"en": {"Confirm", "Cancel"}, "it": {"Conferma", "Annulla"}, "fr": {"Confirmer", "Annuler"},
	"de": {"Bestätigen", "Abbrechen"}, "es": {"Confirmar", "Cancelar"}, "pt": {"Confirmar", "Cancelar"},
	"ar": {"تأكيد", "إلغاء"}, "zh": {"确认", "取消"}, "ko": {"확인", "취소"}, "ja": {"確認", "キャンセル"},
}

var xmlErrorLabels = map[string][2]string{
	"en": {"Invalid XML", "Empty XML"}, "it": {"XML non valido", "XML vuoto"}, "fr": {"XML non valide", "XML vide"},
	"de": {"Ungültiges XML", "Leeres XML"}, "es": {"XML no válido", "XML vacío"}, "pt": {"XML inválido", "XML vazio"},
	"ar": {"XML غير صالح", "XML فارغ"}, "zh": {"XML 无效", "XML 为空"}, "ko": {"잘못된 XML", "빈 XML"}, "ja": {"無効なXML", "空のXML"},
}

var commandEditorTranslations = map[string]commandEditorMessages{
	"en": {Title: "Program editor", CommandsTab: "Text commands", XMLTab: "Advanced XML", Help: "One command per line · distances and altitude in cm · angles in degrees · WAIT in seconds", Unavailable: "Text view unavailable: %v. Use Advanced XML.", Invalid: "Invalid text commands: %v", Sync: "Validate / sync", TextToXML: "Commands are valid. XML regenerated.", XMLToText: "XML is valid. Text commands regenerated.", Save: "Save program"},
	"it": {Title: "Editor programma", CommandsTab: "Comandi testuali", XMLTab: "XML avanzato", Help: "Un comando per riga · distanze e quota in cm · angoli in gradi · WAIT in secondi", Unavailable: "Vista testuale non disponibile: %v. Usa XML avanzato.", Invalid: "Comandi testuali non validi: %v", Sync: "Verifica / sincronizza", TextToXML: "Comandi validi. XML rigenerato.", XMLToText: "XML valido. Comandi testuali rigenerati.", Save: "Salva programma"},
	"fr": {Title: "Éditeur de programme", CommandsTab: "Commandes texte", XMLTab: "XML avancé", Help: "Une commande par ligne · distances et altitude en cm · angles en degrés · WAIT en secondes", Unavailable: "Vue texte indisponible : %v. Utilisez XML avancé.", Invalid: "Commandes texte invalides : %v", Sync: "Valider / synchroniser", TextToXML: "Commandes valides. XML régénéré.", XMLToText: "XML valide. Commandes texte régénérées.", Save: "Enregistrer le programme"},
	"de": {Title: "Programmeditor", CommandsTab: "Textbefehle", XMLTab: "Erweitertes XML", Help: "Ein Befehl pro Zeile · Entfernungen und Höhe in cm · Winkel in Grad · WAIT in Sekunden", Unavailable: "Textansicht nicht verfügbar: %v. Erweitertes XML verwenden.", Invalid: "Ungültige Textbefehle: %v", Sync: "Prüfen / synchronisieren", TextToXML: "Befehle gültig. XML neu erzeugt.", XMLToText: "XML gültig. Textbefehle neu erzeugt.", Save: "Programm speichern"},
	"es": {Title: "Editor de programa", CommandsTab: "Comandos de texto", XMLTab: "XML avanzado", Help: "Un comando por línea · distancias y altitud en cm · ángulos en grados · WAIT en segundos", Unavailable: "Vista de texto no disponible: %v. Usa XML avanzado.", Invalid: "Comandos de texto no válidos: %v", Sync: "Validar / sincronizar", TextToXML: "Comandos válidos. XML regenerado.", XMLToText: "XML válido. Comandos de texto regenerados.", Save: "Guardar programa"},
	"pt": {Title: "Editor de programa", CommandsTab: "Comandos de texto", XMLTab: "XML avançado", Help: "Um comando por linha · distâncias e altitude em cm · ângulos em graus · WAIT em segundos", Unavailable: "Visualização de texto indisponível: %v. Use XML avançado.", Invalid: "Comandos de texto inválidos: %v", Sync: "Validar / sincronizar", TextToXML: "Comandos válidos. XML regenerado.", XMLToText: "XML válido. Comandos de texto regenerados.", Save: "Salvar programa"},
	"ar": {Title: "محرر البرنامج", CommandsTab: "أوامر نصية", XMLTab: "XML متقدم", Help: "أمر واحد في كل سطر · المسافات والارتفاع بالسنتيمتر · الزوايا بالدرجات · WAIT بالثواني", Unavailable: "العرض النصي غير متاح: %v. استخدم XML المتقدم.", Invalid: "الأوامر النصية غير صالحة: %v", Sync: "تحقق / مزامنة", TextToXML: "الأوامر صالحة. أُعيد إنشاء XML.", XMLToText: "XML صالح. أُعيد إنشاء الأوامر النصية.", Save: "حفظ البرنامج"},
	"zh": {Title: "程序编辑器", CommandsTab: "文本命令", XMLTab: "高级 XML", Help: "每行一个命令 · 距离和高度单位为厘米 · 角度单位为度 · WAIT单位为秒", Unavailable: "文本视图不可用：%v。请使用高级 XML。", Invalid: "文本命令无效：%v", Sync: "验证 / 同步", TextToXML: "命令有效。XML 已重新生成。", XMLToText: "XML 有效。文本命令已重新生成。", Save: "保存程序"},
	"ko": {Title: "프로그램 편집기", CommandsTab: "텍스트 명령", XMLTab: "고급 XML", Help: "한 줄에 명령 하나 · 거리와 고도는 cm · 각도는 도 · WAIT는 초", Unavailable: "텍스트 보기를 사용할 수 없음: %v. 고급 XML을 사용하세요.", Invalid: "잘못된 텍스트 명령: %v", Sync: "검사 / 동기화", TextToXML: "명령이 유효합니다. XML을 다시 생성했습니다.", XMLToText: "XML이 유효합니다. 텍스트 명령을 다시 생성했습니다.", Save: "프로그램 저장"},
	"ja": {Title: "プログラムエディター", CommandsTab: "テキストコマンド", XMLTab: "高度なXML", Help: "1行に1コマンド · 距離と高度はcm · 角度は度 · WAITは秒", Unavailable: "テキスト表示を使用できません：%v。高度なXMLを使用してください。", Invalid: "テキストコマンドが無効です：%v", Sync: "検証 / 同期", TextToXML: "コマンドは有効です。XMLを再生成しました。", XMLToText: "XMLは有効です。テキストコマンドを再生成しました。", Save: "プログラムを保存"},
}

var summaryWarningTranslations = map[string][2]string{
	"en": {"The program contains no drone commands.", "The program has no Land block; keep automatic landing enabled."},
	"it": {"Il programma non contiene comandi per il drone.", "Il programma non contiene Atterra; lascia attivo l'atterraggio automatico."},
	"fr": {"Le programme ne contient aucune commande pour le drone.", "Le programme ne contient pas de bloc Atterrir ; laissez l’atterrissage automatique activé."},
	"de": {"Das Programm enthält keine Drohnenbefehle.", "Das Programm enthält keinen Landen-Block; automatische Landung aktiviert lassen."},
	"es": {"El programa no contiene comandos para el dron.", "El programa no contiene un bloque Aterrizar; deja activado el aterrizaje automático."},
	"pt": {"O programa não contém comandos para o drone.", "O programa não contém um bloco Pousar; mantenha o pouso automático ativado."},
	"ar": {"لا يحتوي البرنامج على أوامر للطائرة.", "لا يحتوي البرنامج على كتلة هبوط؛ أبقِ الهبوط التلقائي مفعّلاً."},
	"zh": {"程序中没有无人机命令。", "程序中没有降落积木；请保持自动降落开启。"},
	"ko": {"프로그램에 드론 명령이 없습니다.", "프로그램에 착륙 블록이 없습니다. 자동 착륙을 켜 두세요."},
	"ja": {"プログラムにドローンのコマンドがありません。", "プログラムに着陸ブロックがありません。自動着陸を有効にしてください。"},
}

var uiMessages = map[string]map[string]string{
	"en": {
		"language": "Language", "not_connected": "● Not connected", "simulation": "Simulation (no real commands)",
		"connect": "Connect", "disconnect": "Disconnect", "connection": "Connection", "telemetry": "Telemetry",
		"battery": "BATTERY", "altitude": "ALTITUDE", "heading": "HEADING", "flight_time": "FLIGHT",
		"no_program": "No program selected", "load_program": "Load a Drone Commander XML file.", "choose_xml": "Choose XML",
		"view_edit": "View / edit", "program": "Program", "battery_range": "use a value between 0 and 100",
		"auto_land": "Land automatically when finished", "collision_check": "Collision check", "unit_help": "1 unit = 1 cm · minimum linear movement 20 cm",
		"minimum_battery": "Minimum battery (%)", "start": "Start", "stop": "Stop", "land": "Land", "emergency": "EMERGENCY",
		"motor_warning": "MOTOR STOP: emergency makes the drone fall immediately.", "flight": "Flight",
		"clear_log": "Clear log", "log_help": "Commands, responses and analysis for each STEP", "flight_log": "Flight log",
		"camera": "Camera", "camera_toggle": "Enable camera", "camera_off": "Camera off", "camera_waiting": "Waiting for video…", "camera_live": "Live video",
		"camera_unavailable": "Connect a real Tello to use the camera.", "camera_simulation": "Camera unavailable in simulation.",
		"camera_starting": "Starting camera…", "camera_stopping": "Stopping camera…",
		"media_folder": "Photo/video folder", "choose_media_folder": "Change…",
		"media_folder_prompt": "Choose where to save photos and recordings for this run", "select_folder": "Select folder",
		"local_media_folder": "Choose a local folder for photos and recordings.",
		"select_xml":         "select an XML file first", "xml_not_saved": "XML not saved: %v", "open_write": "open file for writing: %v",
		"write_xml": "write XML: %v", "close_xml": "close XML: %v", "saved_not_reloaded": "XML saved but not reloaded: %v",
		"xml_editor": "XML editor", "xml_auto": "XML is indented automatically. The current flight is not changed.",
		"line_column": "Line %d · Column %d", "reload": "Reload", "reread_formatted": "File reloaded from disk and formatted.",
		"format_xml": "Format XML", "formatted": "XML formatted: two spaces per level.", "save_xml": "Save XML",
		"saved_reloaded": "Saved and reloaded in the driver.", "saved_next": "Saved. Changes will apply on the next start.",
		"saved_other": "Saved. This file is no longer selected in the driver.", "close": "Close", "reread_xml": "reload XML: %v",
		"emergency_title": "Immediate motor stop", "emergency_question": "The Tello will fall immediately. Really send EMERGENCY?",
		"clear_title": "Clear log", "clear_question": "Clear the displayed log and the local log file?", "clear_error": "clear log: %v",
		"connected_simulation": "● Simulation", "connected_tello": "● Tello connected", "program_meta": "%d blocks · %d drone commands",
		"log_empty": "The log is empty. New events will appear here.", "command": "COMMAND", "response": "RESPONSE",
		"error": "ERROR", "analysis": "ANALYSIS", "telemetry_log": "TELEMETRY", "status": "STATUS",
	},
	"it": {
		"language": "Lingua", "not_connected": "● Non connesso", "simulation": "Simulazione (nessun comando reale)",
		"connect": "Connetti", "disconnect": "Disconnetti", "connection": "Connessione", "telemetry": "Telemetria",
		"battery": "BATTERIA", "altitude": "QUOTA", "heading": "ROTTA", "flight_time": "VOLO",
		"no_program": "Nessun programma selezionato", "load_program": "Carica un XML di Drone Commander.", "choose_xml": "Scegli XML",
		"view_edit": "Vedi / modifica", "program": "Programma", "battery_range": "usa un valore tra 0 e 100",
		"auto_land": "Atterra al termine", "collision_check": "Controllo collisioni", "unit_help": "1 unità = 1 cm · movimenti lineari minimi 20 cm",
		"minimum_battery": "Batteria minima (%)", "start": "Avvia", "stop": "Stop", "land": "Atterra", "emergency": "EMERGENZA",
		"motor_warning": "ARRESTO MOTORI: in emergenza il drone cade immediatamente.", "flight": "Volo",
		"clear_log": "Cancella log", "log_help": "Comandi, risposte e analisi di ogni STEP", "flight_log": "Registro di volo",
		"camera": "Telecamera", "camera_toggle": "Attiva telecamera", "camera_off": "Telecamera disattivata", "camera_waiting": "In attesa del video…", "camera_live": "Video in diretta",
		"camera_unavailable": "Connetti un Tello reale per usare la telecamera.", "camera_simulation": "Telecamera non disponibile in simulazione.",
		"camera_starting": "Attivazione telecamera…", "camera_stopping": "Disattivazione telecamera…",
		"media_folder": "Cartella foto/video", "choose_media_folder": "Cambia…",
		"media_folder_prompt": "Scegli dove salvare foto e registrazioni per questa esecuzione", "select_folder": "Seleziona cartella",
		"local_media_folder": "Scegli una cartella locale per foto e registrazioni.",
		"select_xml":         "seleziona prima un file XML", "xml_not_saved": "XML non salvato: %v", "open_write": "apertura file in scrittura: %v",
		"write_xml": "scrittura XML: %v", "close_xml": "chiusura XML: %v", "saved_not_reloaded": "XML salvato ma non ricaricato: %v",
		"xml_editor": "Editor XML", "xml_auto": "XML indentato automaticamente. Il volo in corso non viene alterato.",
		"line_column": "Riga %d · Colonna %d", "reload": "Rileggi", "reread_formatted": "File riletto dal disco e formattato.",
		"format_xml": "Formatta XML", "formatted": "XML formattato: due spazi per ogni livello.", "save_xml": "Salva XML",
		"saved_reloaded": "Salvato e ricaricato nel driver.", "saved_next": "Salvato. Le modifiche saranno applicate al prossimo avvio.",
		"saved_other": "Salvato. Questo file non è più quello selezionato nel driver.", "close": "Chiudi", "reread_xml": "rilettura XML: %v",
		"emergency_title": "Arresto immediato dei motori", "emergency_question": "Il Tello cadrà immediatamente. Inviare davvero EMERGENCY?",
		"clear_title": "Cancella registro", "clear_question": "Cancellare il registro mostrato e il file di log locale?", "clear_error": "cancellazione log: %v",
		"connected_simulation": "● Simulazione", "connected_tello": "● Tello connesso", "program_meta": "%d blocchi · %d comandi drone",
		"log_empty": "Registro vuoto. I nuovi eventi appariranno qui.", "command": "COMANDO", "response": "RISPOSTA",
		"error": "ERRORE", "analysis": "ANALISI", "telemetry_log": "TELEMETRIA", "status": "STATO",
	},
	"fr": {
		"language": "Langue", "not_connected": "● Non connecté", "simulation": "Simulation (aucune commande réelle)",
		"connect": "Connecter", "disconnect": "Déconnecter", "connection": "Connexion", "telemetry": "Télémétrie",
		"battery": "BATTERIE", "altitude": "ALTITUDE", "heading": "CAP", "flight_time": "VOL",
		"no_program": "Aucun programme sélectionné", "load_program": "Chargez un fichier XML Drone Commander.", "choose_xml": "Choisir XML",
		"view_edit": "Voir / modifier", "program": "Programme", "battery_range": "utilisez une valeur entre 0 et 100",
		"auto_land": "Atterrir automatiquement à la fin", "collision_check": "Contrôle des collisions", "unit_help": "1 unité = 1 cm · déplacement linéaire minimal 20 cm",
		"minimum_battery": "Batterie minimale (%)", "start": "Démarrer", "stop": "Stop", "land": "Atterrir", "emergency": "URGENCE",
		"motor_warning": "ARRÊT MOTEURS : en urgence, le drone tombe immédiatement.", "flight": "Vol",
		"clear_log": "Effacer le journal", "log_help": "Commandes, réponses et analyse de chaque ÉTAPE", "flight_log": "Journal de vol",
		"camera": "Caméra", "camera_toggle": "Activer la caméra", "camera_off": "Caméra désactivée", "camera_waiting": "En attente de la vidéo…", "camera_live": "Vidéo en direct",
		"camera_unavailable": "Connectez un Tello réel pour utiliser la caméra.", "camera_simulation": "Caméra indisponible en simulation.",
		"camera_starting": "Activation de la caméra…", "camera_stopping": "Désactivation de la caméra…",
		"media_folder": "Dossier photos/vidéos", "choose_media_folder": "Modifier…",
		"media_folder_prompt": "Choisissez où enregistrer les photos et vidéos pour cette exécution", "select_folder": "Sélectionner le dossier",
		"local_media_folder": "Choisissez un dossier local pour les photos et vidéos.",
		"select_xml":         "sélectionnez d’abord un fichier XML", "xml_not_saved": "XML non enregistré : %v", "open_write": "ouverture du fichier en écriture : %v",
		"write_xml": "écriture XML : %v", "close_xml": "fermeture XML : %v", "saved_not_reloaded": "XML enregistré mais non rechargé : %v",
		"xml_editor": "Éditeur XML", "xml_auto": "Le XML est indenté automatiquement. Le vol en cours n’est pas modifié.",
		"line_column": "Ligne %d · Colonne %d", "reload": "Recharger", "reread_formatted": "Fichier relu depuis le disque et formaté.",
		"format_xml": "Formater XML", "formatted": "XML formaté : deux espaces par niveau.", "save_xml": "Enregistrer XML",
		"saved_reloaded": "Enregistré et rechargé dans le pilote.", "saved_next": "Enregistré. Les modifications s’appliqueront au prochain démarrage.",
		"saved_other": "Enregistré. Ce fichier n’est plus sélectionné dans le pilote.", "close": "Fermer", "reread_xml": "rechargement XML : %v",
		"emergency_title": "Arrêt immédiat des moteurs", "emergency_question": "Le Tello tombera immédiatement. Envoyer vraiment EMERGENCY ?",
		"clear_title": "Effacer le journal", "clear_question": "Effacer le journal affiché et le fichier local ?", "clear_error": "effacement du journal : %v",
		"connected_simulation": "● Simulation", "connected_tello": "● Tello connecté", "program_meta": "%d blocs · %d commandes drone",
		"log_empty": "Le journal est vide. Les nouveaux événements apparaîtront ici.", "command": "COMMANDE", "response": "RÉPONSE",
		"error": "ERREUR", "analysis": "ANALYSE", "telemetry_log": "TÉLÉMÉTRIE", "status": "ÉTAT",
	},
	"de": {
		"language": "Sprache", "not_connected": "● Nicht verbunden", "simulation": "Simulation (keine echten Befehle)",
		"connect": "Verbinden", "disconnect": "Trennen", "connection": "Verbindung", "telemetry": "Telemetrie",
		"battery": "AKKU", "altitude": "HÖHE", "heading": "KURS", "flight_time": "FLUG",
		"no_program": "Kein Programm ausgewählt", "load_program": "Drone-Commander-XML laden.", "choose_xml": "XML wählen",
		"view_edit": "Anzeigen / bearbeiten", "program": "Programm", "battery_range": "Wert zwischen 0 und 100 verwenden",
		"auto_land": "Nach Abschluss automatisch landen", "collision_check": "Kollisionskontrolle", "unit_help": "1 Einheit = 1 cm · lineare Mindestbewegung 20 cm",
		"minimum_battery": "Mindestakku (%)", "start": "Starten", "stop": "Stop", "land": "Landen", "emergency": "NOTFALL",
		"motor_warning": "MOTORSTOPP: Im Notfall fällt die Drohne sofort.", "flight": "Flug",
		"clear_log": "Protokoll löschen", "log_help": "Befehle, Antworten und Analyse jedes SCHRITTS", "flight_log": "Flugprotokoll",
		"camera": "Kamera", "camera_toggle": "Kamera aktivieren", "camera_off": "Kamera aus", "camera_waiting": "Warten auf Video…", "camera_live": "Live-Video",
		"camera_unavailable": "Verbinde einen echten Tello, um die Kamera zu verwenden.", "camera_simulation": "Kamera in der Simulation nicht verfügbar.",
		"camera_starting": "Kamera wird aktiviert…", "camera_stopping": "Kamera wird deaktiviert…",
		"media_folder": "Foto-/Videoordner", "choose_media_folder": "Ändern…",
		"media_folder_prompt": "Speicherort für Fotos und Aufnahmen dieses Laufs wählen", "select_folder": "Ordner auswählen",
		"local_media_folder": "Wähle einen lokalen Ordner für Fotos und Aufnahmen.",
		"select_xml":         "zuerst eine XML-Datei auswählen", "xml_not_saved": "XML nicht gespeichert: %v", "open_write": "Datei zum Schreiben öffnen: %v",
		"write_xml": "XML schreiben: %v", "close_xml": "XML schließen: %v", "saved_not_reloaded": "XML gespeichert, aber nicht neu geladen: %v",
		"xml_editor": "XML-Editor", "xml_auto": "XML wird automatisch eingerückt. Der laufende Flug bleibt unverändert.",
		"line_column": "Zeile %d · Spalte %d", "reload": "Neu laden", "reread_formatted": "Datei vom Datenträger neu geladen und formatiert.",
		"format_xml": "XML formatieren", "formatted": "XML formatiert: zwei Leerzeichen pro Ebene.", "save_xml": "XML speichern",
		"saved_reloaded": "Gespeichert und im Treiber neu geladen.", "saved_next": "Gespeichert. Änderungen gelten beim nächsten Start.",
		"saved_other": "Gespeichert. Diese Datei ist im Treiber nicht mehr ausgewählt.", "close": "Schließen", "reread_xml": "XML neu laden: %v",
		"emergency_title": "Sofortiger Motorstopp", "emergency_question": "Der Tello fällt sofort. EMERGENCY wirklich senden?",
		"clear_title": "Protokoll löschen", "clear_question": "Angezeigtes Protokoll und lokale Protokolldatei löschen?", "clear_error": "Protokoll löschen: %v",
		"connected_simulation": "● Simulation", "connected_tello": "● Tello verbunden", "program_meta": "%d Blöcke · %d Drohnenbefehle",
		"log_empty": "Das Protokoll ist leer. Neue Ereignisse erscheinen hier.", "command": "BEFEHL", "response": "ANTWORT",
		"error": "FEHLER", "analysis": "ANALYSE", "telemetry_log": "TELEMETRIE", "status": "STATUS",
	},
	"es": {
		"language": "Idioma", "not_connected": "● Sin conexión", "simulation": "Simulación (sin comandos reales)",
		"connect": "Conectar", "disconnect": "Desconectar", "connection": "Conexión", "telemetry": "Telemetría",
		"battery": "BATERÍA", "altitude": "ALTITUD", "heading": "RUMBO", "flight_time": "VUELO",
		"no_program": "Ningún programa seleccionado", "load_program": "Carga un XML de Drone Commander.", "choose_xml": "Elegir XML",
		"view_edit": "Ver / editar", "program": "Programa", "battery_range": "usa un valor entre 0 y 100",
		"auto_land": "Aterrizar automáticamente al finalizar", "collision_check": "Control de colisiones", "unit_help": "1 unidad = 1 cm · movimiento lineal mínimo 20 cm",
		"minimum_battery": "Batería mínima (%)", "start": "Iniciar", "stop": "Stop", "land": "Aterrizar", "emergency": "EMERGENCIA",
		"motor_warning": "PARADA DE MOTORES: en emergencia el dron cae inmediatamente.", "flight": "Vuelo",
		"clear_log": "Borrar registro", "log_help": "Comandos, respuestas y análisis de cada PASO", "flight_log": "Registro de vuelo",
		"camera": "Cámara", "camera_toggle": "Activar cámara", "camera_off": "Cámara desactivada", "camera_waiting": "Esperando vídeo…", "camera_live": "Vídeo en directo",
		"camera_unavailable": "Conecta un Tello real para usar la cámara.", "camera_simulation": "Cámara no disponible en simulación.",
		"camera_starting": "Activando cámara…", "camera_stopping": "Desactivando cámara…",
		"media_folder": "Carpeta de fotos/vídeos", "choose_media_folder": "Cambiar…",
		"media_folder_prompt": "Elige dónde guardar fotos y grabaciones de esta ejecución", "select_folder": "Seleccionar carpeta",
		"local_media_folder": "Elige una carpeta local para fotos y grabaciones.",
		"select_xml":         "selecciona primero un archivo XML", "xml_not_saved": "XML no guardado: %v", "open_write": "abrir archivo para escritura: %v",
		"write_xml": "escribir XML: %v", "close_xml": "cerrar XML: %v", "saved_not_reloaded": "XML guardado pero no recargado: %v",
		"xml_editor": "Editor XML", "xml_auto": "El XML se indenta automáticamente. El vuelo actual no cambia.",
		"line_column": "Línea %d · Columna %d", "reload": "Recargar", "reread_formatted": "Archivo recargado del disco y formateado.",
		"format_xml": "Formatear XML", "formatted": "XML formateado: dos espacios por nivel.", "save_xml": "Guardar XML",
		"saved_reloaded": "Guardado y recargado en el controlador.", "saved_next": "Guardado. Los cambios se aplicarán en el próximo inicio.",
		"saved_other": "Guardado. Este archivo ya no está seleccionado en el controlador.", "close": "Cerrar", "reread_xml": "recargar XML: %v",
		"emergency_title": "Parada inmediata de motores", "emergency_question": "El Tello caerá inmediatamente. ¿Enviar EMERGENCY?",
		"clear_title": "Borrar registro", "clear_question": "¿Borrar el registro mostrado y el archivo local?", "clear_error": "borrar registro: %v",
		"connected_simulation": "● Simulación", "connected_tello": "● Tello conectado", "program_meta": "%d bloques · %d comandos de dron",
		"log_empty": "El registro está vacío. Los nuevos eventos aparecerán aquí.", "command": "COMANDO", "response": "RESPUESTA",
		"error": "ERROR", "analysis": "ANÁLISIS", "telemetry_log": "TELEMETRÍA", "status": "ESTADO",
	},
	"pt": {
		"language": "Idioma", "not_connected": "● Não conectado", "simulation": "Simulação (sem comandos reais)",
		"connect": "Conectar", "disconnect": "Desconectar", "connection": "Conexão", "telemetry": "Telemetria",
		"battery": "BATERIA", "altitude": "ALTITUDE", "heading": "RUMO", "flight_time": "VOO",
		"no_program": "Nenhum programa selecionado", "load_program": "Carregue um XML do Drone Commander.", "choose_xml": "Escolher XML",
		"view_edit": "Ver / editar", "program": "Programa", "battery_range": "use um valor entre 0 e 100",
		"auto_land": "Pousar automaticamente ao terminar", "collision_check": "Controle de colisões", "unit_help": "1 unidade = 1 cm · movimento linear mínimo 20 cm",
		"minimum_battery": "Bateria mínima (%)", "start": "Iniciar", "stop": "Stop", "land": "Pousar", "emergency": "EMERGÊNCIA",
		"motor_warning": "PARADA DOS MOTORES: em emergência o drone cai imediatamente.", "flight": "Voo",
		"clear_log": "Limpar registro", "log_help": "Comandos, respostas e análise de cada ETAPA", "flight_log": "Registro de voo",
		"camera": "Câmera", "camera_toggle": "Ativar câmera", "camera_off": "Câmera desativada", "camera_waiting": "Aguardando vídeo…", "camera_live": "Vídeo ao vivo",
		"camera_unavailable": "Conecte um Tello real para usar a câmera.", "camera_simulation": "Câmera indisponível na simulação.",
		"camera_starting": "Ativando câmera…", "camera_stopping": "Desativando câmera…",
		"media_folder": "Pasta de fotos/vídeos", "choose_media_folder": "Alterar…",
		"media_folder_prompt": "Escolha onde salvar fotos e gravações desta execução", "select_folder": "Selecionar pasta",
		"local_media_folder": "Escolha uma pasta local para fotos e gravações.",
		"select_xml":         "selecione primeiro um arquivo XML", "xml_not_saved": "XML não salvo: %v", "open_write": "abrir arquivo para escrita: %v",
		"write_xml": "gravar XML: %v", "close_xml": "fechar XML: %v", "saved_not_reloaded": "XML salvo, mas não recarregado: %v",
		"xml_editor": "Editor XML", "xml_auto": "O XML é indentado automaticamente. O voo atual não é alterado.",
		"line_column": "Linha %d · Coluna %d", "reload": "Recarregar", "reread_formatted": "Arquivo recarregado do disco e formatado.",
		"format_xml": "Formatar XML", "formatted": "XML formatado: dois espaços por nível.", "save_xml": "Salvar XML",
		"saved_reloaded": "Salvo e recarregado no driver.", "saved_next": "Salvo. As alterações serão aplicadas na próxima execução.",
		"saved_other": "Salvo. Este arquivo não está mais selecionado no driver.", "close": "Fechar", "reread_xml": "recarregar XML: %v",
		"emergency_title": "Parada imediata dos motores", "emergency_question": "O Tello cairá imediatamente. Enviar EMERGENCY?",
		"clear_title": "Limpar registro", "clear_question": "Limpar o registro exibido e o arquivo local?", "clear_error": "limpar registro: %v",
		"connected_simulation": "● Simulação", "connected_tello": "● Tello conectado", "program_meta": "%d blocos · %d comandos do drone",
		"log_empty": "O registro está vazio. Novos eventos aparecerão aqui.", "command": "COMANDO", "response": "RESPOSTA",
		"error": "ERRO", "analysis": "ANÁLISE", "telemetry_log": "TELEMETRIA", "status": "ESTADO",
	},
	"ar": {
		"language": "اللغة", "not_connected": "● غير متصل", "simulation": "محاكاة (بدون أوامر حقيقية)",
		"connect": "اتصال", "disconnect": "قطع الاتصال", "connection": "الاتصال", "telemetry": "القياسات",
		"battery": "البطارية", "altitude": "الارتفاع", "heading": "الاتجاه", "flight_time": "الطيران",
		"no_program": "لم يتم اختيار برنامج", "load_program": "حمّل ملف XML من Drone Commander.", "choose_xml": "اختيار XML",
		"view_edit": "عرض / تعديل", "program": "البرنامج", "battery_range": "استخدم قيمة بين 0 و100",
		"auto_land": "الهبوط تلقائياً عند الانتهاء", "collision_check": "التحقق من الاصطدامات", "unit_help": "وحدة واحدة = 1 سم · الحد الأدنى للحركة الخطية 20 سم",
		"minimum_battery": "الحد الأدنى للبطارية (%)", "start": "بدء", "stop": "إيقاف", "land": "هبوط", "emergency": "طوارئ",
		"motor_warning": "إيقاف المحركات: في الطوارئ تسقط الطائرة فوراً.", "flight": "الطيران",
		"clear_log": "مسح السجل", "log_help": "الأوامر والردود وتحليل كل خطوة", "flight_log": "سجل الطيران",
		"camera": "الكاميرا", "camera_toggle": "تفعيل الكاميرا", "camera_off": "الكاميرا متوقفة", "camera_waiting": "بانتظار الفيديو…", "camera_live": "فيديو مباشر",
		"camera_unavailable": "اتصل بطائرة Tello حقيقية لاستخدام الكاميرا.", "camera_simulation": "الكاميرا غير متاحة في المحاكاة.",
		"camera_starting": "جارٍ تفعيل الكاميرا…", "camera_stopping": "جارٍ إيقاف الكاميرا…",
		"media_folder": "مجلد الصور/الفيديو", "choose_media_folder": "تغيير…",
		"media_folder_prompt": "اختر مكان حفظ الصور والتسجيلات لهذا التشغيل", "select_folder": "اختيار المجلد",
		"local_media_folder": "اختر مجلدًا محليًا للصور والتسجيلات.",
		"select_xml":         "اختر ملف XML أولاً", "xml_not_saved": "لم يُحفظ XML: %v", "open_write": "فتح الملف للكتابة: %v",
		"write_xml": "كتابة XML: %v", "close_xml": "إغلاق XML: %v", "saved_not_reloaded": "حُفظ XML ولكن لم يُعد تحميله: %v",
		"xml_editor": "محرر XML", "xml_auto": "يتم ترتيب XML تلقائياً. لن تتغير الرحلة الحالية.",
		"line_column": "السطر %d · العمود %d", "reload": "إعادة تحميل", "reread_formatted": "أُعيد تحميل الملف وتنسيقه.",
		"format_xml": "تنسيق XML", "formatted": "تم تنسيق XML: مسافتان لكل مستوى.", "save_xml": "حفظ XML",
		"saved_reloaded": "تم الحفظ وإعادة التحميل في برنامج التشغيل.", "saved_next": "تم الحفظ. ستطبق التغييرات عند التشغيل التالي.",
		"saved_other": "تم الحفظ. لم يعد هذا الملف محدداً.", "close": "إغلاق", "reread_xml": "إعادة تحميل XML: %v",
		"emergency_title": "إيقاف المحركات فوراً", "emergency_question": "سيسقط Tello فوراً. هل تريد إرسال EMERGENCY؟",
		"clear_title": "مسح السجل", "clear_question": "هل تريد مسح السجل المعروض والملف المحلي؟", "clear_error": "مسح السجل: %v",
		"connected_simulation": "● محاكاة", "connected_tello": "● Tello متصل", "program_meta": "%d كتلة · %d أمر للطائرة",
		"log_empty": "السجل فارغ. ستظهر الأحداث الجديدة هنا.", "command": "أمر", "response": "رد",
		"error": "خطأ", "analysis": "تحليل", "telemetry_log": "قياسات", "status": "حالة",
	},
	"zh": {
		"language": "语言", "not_connected": "● 未连接", "simulation": "模拟（不发送真实命令）",
		"connect": "连接", "disconnect": "断开连接", "connection": "连接", "telemetry": "遥测",
		"battery": "电池", "altitude": "高度", "heading": "航向", "flight_time": "飞行",
		"no_program": "未选择程序", "load_program": "加载 Drone Commander XML 文件。", "choose_xml": "选择 XML",
		"view_edit": "查看 / 编辑", "program": "程序", "battery_range": "请输入 0 到 100 之间的值",
		"auto_land": "结束后自动降落", "collision_check": "碰撞检查", "unit_help": "1 单位 = 1 厘米 · 最小直线移动 20 厘米",
		"minimum_battery": "最低电量 (%)", "start": "开始", "stop": "停止", "land": "降落", "emergency": "紧急停止",
		"motor_warning": "电机急停：无人机会立即坠落。", "flight": "飞行",
		"clear_log": "清除日志", "log_help": "每个步骤的命令、响应和分析", "flight_log": "飞行日志",
		"camera": "摄像头", "camera_toggle": "启用摄像头", "camera_off": "摄像头已关闭", "camera_waiting": "正在等待视频…", "camera_live": "实时视频",
		"camera_unavailable": "连接真实 Tello 后可使用摄像头。", "camera_simulation": "模拟模式下无法使用摄像头。",
		"camera_starting": "正在启用摄像头…", "camera_stopping": "正在关闭摄像头…",
		"media_folder": "照片/视频文件夹", "choose_media_folder": "更改…",
		"media_folder_prompt": "选择本次运行保存照片和录像的位置", "select_folder": "选择文件夹",
		"local_media_folder": "请选择用于保存照片和录像的本地文件夹。",
		"select_xml":         "请先选择 XML 文件", "xml_not_saved": "XML 未保存：%v", "open_write": "打开文件写入：%v",
		"write_xml": "写入 XML：%v", "close_xml": "关闭 XML：%v", "saved_not_reloaded": "XML 已保存但未重新加载：%v",
		"xml_editor": "XML 编辑器", "xml_auto": "XML 会自动缩进。当前飞行不会改变。",
		"line_column": "第 %d 行 · 第 %d 列", "reload": "重新加载", "reread_formatted": "文件已从磁盘重新加载并格式化。",
		"format_xml": "格式化 XML", "formatted": "XML 已格式化：每级两个空格。", "save_xml": "保存 XML",
		"saved_reloaded": "已保存并重新加载到驱动程序。", "saved_next": "已保存。更改将在下次启动时应用。",
		"saved_other": "已保存。驱动程序中已不再选择此文件。", "close": "关闭", "reread_xml": "重新加载 XML：%v",
		"emergency_title": "立即停止电机", "emergency_question": "Tello 会立即坠落。确定发送 EMERGENCY？",
		"clear_title": "清除日志", "clear_question": "清除显示的日志和本地日志文件？", "clear_error": "清除日志：%v",
		"connected_simulation": "● 模拟", "connected_tello": "● Tello 已连接", "program_meta": "%d 个积木 · %d 个无人机命令",
		"log_empty": "日志为空。新事件将显示在此处。", "command": "命令", "response": "响应",
		"error": "错误", "analysis": "分析", "telemetry_log": "遥测", "status": "状态",
	},
	"ko": {
		"language": "언어", "not_connected": "● 연결되지 않음", "simulation": "시뮬레이션 (실제 명령 없음)",
		"connect": "연결", "disconnect": "연결 해제", "connection": "연결", "telemetry": "텔레메트리",
		"battery": "배터리", "altitude": "고도", "heading": "방향", "flight_time": "비행",
		"no_program": "선택된 프로그램 없음", "load_program": "Drone Commander XML 파일을 불러오세요.", "choose_xml": "XML 선택",
		"view_edit": "보기 / 편집", "program": "프로그램", "battery_range": "0에서 100 사이의 값을 사용하세요",
		"auto_land": "완료 후 자동 착륙", "collision_check": "충돌 검사", "unit_help": "1 단위 = 1cm · 최소 직선 이동 20cm",
		"minimum_battery": "최소 배터리 (%)", "start": "시작", "stop": "정지", "land": "착륙", "emergency": "비상 정지",
		"motor_warning": "모터 정지: 비상 시 드론이 즉시 추락합니다.", "flight": "비행",
		"clear_log": "로그 지우기", "log_help": "각 단계의 명령, 응답 및 분석", "flight_log": "비행 로그",
		"camera": "카메라", "camera_toggle": "카메라 활성화", "camera_off": "카메라 꺼짐", "camera_waiting": "비디오 대기 중…", "camera_live": "실시간 비디오",
		"camera_unavailable": "카메라를 사용하려면 실제 Tello를 연결하세요.", "camera_simulation": "시뮬레이션에서는 카메라를 사용할 수 없습니다.",
		"camera_starting": "카메라 활성화 중…", "camera_stopping": "카메라 비활성화 중…",
		"media_folder": "사진/동영상 폴더", "choose_media_folder": "변경…",
		"media_folder_prompt": "이번 실행의 사진과 녹화를 저장할 위치를 선택하세요", "select_folder": "폴더 선택",
		"local_media_folder": "사진과 녹화를 저장할 로컬 폴더를 선택하세요.",
		"select_xml":         "먼저 XML 파일을 선택하세요", "xml_not_saved": "XML이 저장되지 않음: %v", "open_write": "쓰기 위해 파일 열기: %v",
		"write_xml": "XML 쓰기: %v", "close_xml": "XML 닫기: %v", "saved_not_reloaded": "XML은 저장되었지만 다시 불러오지 못함: %v",
		"xml_editor": "XML 편집기", "xml_auto": "XML은 자동으로 들여쓰기됩니다. 현재 비행은 변경되지 않습니다.",
		"line_column": "%d행 · %d열", "reload": "다시 불러오기", "reread_formatted": "파일을 디스크에서 다시 불러와 형식을 맞췄습니다.",
		"format_xml": "XML 서식 지정", "formatted": "XML 형식 지정 완료: 단계마다 공백 두 칸.", "save_xml": "XML 저장",
		"saved_reloaded": "저장하고 드라이버에 다시 불러왔습니다.", "saved_next": "저장됨. 다음 시작부터 변경 사항이 적용됩니다.",
		"saved_other": "저장됨. 이 파일은 더 이상 드라이버에서 선택되지 않았습니다.", "close": "닫기", "reread_xml": "XML 다시 불러오기: %v",
		"emergency_title": "즉시 모터 정지", "emergency_question": "Tello가 즉시 추락합니다. EMERGENCY를 전송할까요?",
		"clear_title": "로그 지우기", "clear_question": "표시된 로그와 로컬 로그 파일을 지울까요?", "clear_error": "로그 지우기: %v",
		"connected_simulation": "● 시뮬레이션", "connected_tello": "● Tello 연결됨", "program_meta": "%d개 블록 · %d개 드론 명령",
		"log_empty": "로그가 비어 있습니다. 새 이벤트가 여기에 표시됩니다.", "command": "명령", "response": "응답",
		"error": "오류", "analysis": "분석", "telemetry_log": "텔레메트리", "status": "상태",
	},
	"ja": {
		"language": "言語", "not_connected": "● 未接続", "simulation": "シミュレーション（実機コマンドなし）",
		"connect": "接続", "disconnect": "切断", "connection": "接続", "telemetry": "テレメトリ",
		"battery": "バッテリー", "altitude": "高度", "heading": "方位", "flight_time": "飛行",
		"no_program": "プログラムが選択されていません", "load_program": "Drone Commander XMLを読み込んでください。", "choose_xml": "XMLを選択",
		"view_edit": "表示 / 編集", "program": "プログラム", "battery_range": "0から100の値を使用してください",
		"auto_land": "終了時に自動着陸", "collision_check": "衝突チェック", "unit_help": "1単位 = 1cm · 直線移動の最小値 20cm",
		"minimum_battery": "最低バッテリー (%)", "start": "開始", "stop": "停止", "land": "着陸", "emergency": "緊急停止",
		"motor_warning": "モーター停止：緊急時はドローンが直ちに落下します。", "flight": "飛行",
		"clear_log": "ログを消去", "log_help": "各ステップのコマンド、応答、解析", "flight_log": "飛行ログ",
		"camera": "カメラ", "camera_toggle": "カメラを有効化", "camera_off": "カメラはオフです", "camera_waiting": "映像を待っています…", "camera_live": "ライブ映像",
		"camera_unavailable": "カメラを使うには実機のTelloに接続してください。", "camera_simulation": "シミュレーションではカメラを使用できません。",
		"camera_starting": "カメラを有効化中…", "camera_stopping": "カメラを無効化中…",
		"media_folder": "写真/動画フォルダー", "choose_media_folder": "変更…",
		"media_folder_prompt": "今回の実行で写真と録画を保存する場所を選択", "select_folder": "フォルダーを選択",
		"local_media_folder": "写真と録画用のローカルフォルダーを選択してください。",
		"select_xml":         "先にXMLファイルを選択してください", "xml_not_saved": "XMLを保存できません：%v", "open_write": "書き込み用にファイルを開く：%v",
		"write_xml": "XML書き込み：%v", "close_xml": "XMLを閉じる：%v", "saved_not_reloaded": "XMLは保存されましたが再読み込みされませんでした：%v",
		"xml_editor": "XMLエディター", "xml_auto": "XMLは自動的にインデントされます。現在の飛行は変更されません。",
		"line_column": "%d行 · %d列", "reload": "再読み込み", "reread_formatted": "ファイルをディスクから再読み込みして整形しました。",
		"format_xml": "XMLを整形", "formatted": "XML整形済み：階層ごとに2スペース。", "save_xml": "XMLを保存",
		"saved_reloaded": "保存してドライバーに再読み込みしました。", "saved_next": "保存しました。次回の開始時に変更が適用されます。",
		"saved_other": "保存しました。このファイルはドライバーで選択されていません。", "close": "閉じる", "reread_xml": "XML再読み込み：%v",
		"emergency_title": "モーターを即時停止", "emergency_question": "Telloは直ちに落下します。EMERGENCYを送信しますか？",
		"clear_title": "ログを消去", "clear_question": "表示ログとローカルログファイルを消去しますか？", "clear_error": "ログ消去：%v",
		"connected_simulation": "● シミュレーション", "connected_tello": "● Tello接続済み", "program_meta": "%dブロック · %dドローンコマンド",
		"log_empty": "ログは空です。新しいイベントがここに表示されます。", "command": "コマンド", "response": "応答",
		"error": "エラー", "analysis": "解析", "telemetry_log": "テレメトリ", "status": "状態",
	},
}

func normalizeLanguage(code string) string {
	code = strings.ToLower(strings.TrimSpace(code))
	if separator := strings.IndexAny(code, "-_"); separator >= 0 {
		code = code[:separator]
	}
	if _, ok := uiMessages[code]; ok {
		return code
	}
	return "en"
}

func languageName(code string) string {
	code = normalizeLanguage(code)
	for _, option := range supportedLanguages {
		if option.Code == code {
			return option.Name
		}
	}
	return supportedLanguages[0].Name
}

func languageCode(name string) string {
	for _, option := range supportedLanguages {
		if option.Name == name {
			return option.Code
		}
	}
	return "en"
}

func tr(language, key string) string {
	language = normalizeLanguage(language)
	if translated := uiMessages[language][key]; translated != "" {
		return translated
	}
	if fallback := uiMessages["en"][key]; fallback != "" {
		return fallback
	}
	return key
}

func confirmLabels(language string) (confirm, cancel string) {
	labels := confirmationLabels[normalizeLanguage(language)]
	return labels[0], labels[1]
}

func translateSummaryWarnings(language string, warnings []string) string {
	translated := make([]string, 0, len(warnings))
	localized := summaryWarningTranslations[normalizeLanguage(language)]
	italian := summaryWarningTranslations["it"]
	for _, warning := range warnings {
		switch warning {
		case italian[0]:
			translated = append(translated, localized[0])
		case italian[1]:
			translated = append(translated, localized[1])
		default:
			translated = append(translated, warning)
		}
	}
	return strings.Join(translated, " ")
}
