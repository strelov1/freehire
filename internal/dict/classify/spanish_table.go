package classify

// spanishCategoryTable lists the Spanish category aliases, whole-word phrases on the
// folded title, in precedence order.
var spanishCategoryTable = []aliasEntry{
	// Band 1: a head and field whose answer is not the field's.

	// The machine shop and the plant, before the developer: a CNC programmer runs a
	// machining centre, a PLC or robot programmer automates a plant (the main table
	// blinds "programista plc" the same way), and a production programmer schedules
	// the line.
	{"programador cnc", "manufacturing"},
	{"programador de cnc", "manufacturing"},
	{"programador cam", "manufacturing"},
	{"programador de maquinas", "manufacturing"},
	{"programador de produccion", "manufacturing"},
	{"programador plc", categoryNone},
	{"programador de plc", categoryNone},
	{"programador de robots", categoryNone},
	{"programador robot", categoryNone},
	{"programador de rutas", "logistics"}, // the route planner
	{"programador de ruta", "logistics"},
	{"programador de producao", "manufacturing"},   // Portuguese shares this fallback
	{"programador de mantenimiento", categoryNone}, // the maintenance planner
	{"programador de manutencao", categoryNone},
	{"programador de servicios", categoryNone}, // a dealership's workshop scheduler
	{"programador de servicos", categoryNone},
	{"programador scada", categoryNone},
	{"programador cultural", categoryNone}, // a venue's programme curator
	{"programador de eventos", categoryNone},
	// The business and the property developer, before the software one.
	{"desarrollador de negocio", "sales"},
	{"desarrollador de negocios", "sales"},
	{"desarrollador comercial", "sales"},
	{"desarrollador de ventas", "sales"},
	{"desarrollador de mercado", "sales"},
	{"desarrollador de clientes", "sales"},
	{"desarrollo de negocio", "sales"}, // the main table's "business development"
	{"desarrollo de negocios", "sales"},
	{"desarrollador inmobiliario", categoryNone},
	{"desarrollador de proyectos", categoryNone}, // an energy or property project developer
	{"desarrollador de producto", categoryNone},  // a food or fashion product developer

	// Information security and occupational safety, before the guard: "seguridad"
	// names all three.
	{"ciberseguridad", "security"},
	{"seguridad informatica", "security"},
	{"seguridad de la informacion", "security"},
	{"seguridad ofensiva", "security"},
	{"analista soc", "security"},
	{"operador soc", "security"},
	{"prevencion de riesgos", "occupational_safety"},
	{"prevencionista", "occupational_safety"},
	{"seguridad y salud", "occupational_safety"},
	{"seguridad e higiene", "occupational_safety"},
	{"higiene y seguridad", "occupational_safety"},
	{"salud ocupacional", "occupational_safety"},
	{"seguridad laboral", "occupational_safety"},
	{"seguridad industrial", "occupational_safety"},
	{"seguridad y medio ambiente", "occupational_safety"},
	{"prevencionistas", "occupational_safety"},

	// IT.
	{"analista programador", "software_engineering"},
	{"ingeniero de datos", "data_engineering"},
	{"ingeniera de datos", "data_engineering"},
	{"analista de datos", "data_analytics"},
	{"cientifico de datos", "data_science"},
	{"cientifica de datos", "data_science"},
	{"analista funcional", "business_analysis"},
	{"analista de negocio", "business_analysis"},
	{"analista de procesos", "business_analysis"},
	{"analista de sistemas", "software_engineering"},
	{"analista de pruebas", "qa"},
	{"ingeniero de pruebas", "qa"},
	{"tester", "qa"},
	{"calidad de software", "qa"},
	{"calidad del software", "qa"},
	{"administrador de sistemas", "system_administration"}, // the main table files the system administrator here
	{"administradora de sistemas", "system_administration"},
	{"tecnico de sistemas", "it_support"},
	{"tecnica de sistemas", "it_support"},
	{"administrador de redes", "network_engineering"},
	{"ingeniero de redes", "network_engineering"},
	{"tecnico de redes", "network_engineering"},
	{"tecnico informatico", "it_support"},
	{"tecnica informatica", "it_support"},
	{"tecnico microinformatico", "it_support"},
	{"tecnico de soporte", "it_support"},
	{"tecnica de soporte", "it_support"},
	{"soporte tecnico", "it_support"},
	{"soporte it", "it_support"},
	{"soporte informatico", "it_support"},
	{"ingeniero de soporte", "it_support"},
	{"ingeniero informatico", "software_engineering"},
	{"ingenieros informaticos", "software_engineering"},
	{"ingeniero de sistemas", "software_engineering"}, // the main table's "systems engineer"
	{"ingeniero de infraestructura", "devops"},
	{"arquitecto de software", "architecture"},
	{"arquitecto de soluciones", "architecture"},
	{"arquitecto cloud", "architecture"},
	{"arquitecto de datos", "architecture"},
	{"arquitecto de sistemas", "architecture"},
	{"consultor sap", "solutions_engineering"}, // the main table's "SAP consultant"
	{"desarrollador", "software_engineering"},
	{"desarrolladora", "software_engineering"},
	{"desarrolladores", "software_engineering"},
	{"programador", "software_engineering"},
	{"programadora", "software_engineering"},
	{"programadores", "software_engineering"},

	// Engineering seats: the engineer of a sales team pre-sells; of a plant, runs it.
	{"ingeniero de ventas", "solutions_engineering"},
	{"ingeniera de ventas", "solutions_engineering"},
	{"ingeniero de preventa", "solutions_engineering"},
	{"preventa", "solutions_engineering"},
	{"ingeniero de mantenimiento", "industrial_engineering"},
	{"ingeniero mantenimiento", "industrial_engineering"},
	{"ingeniero de procesos", "industrial_engineering"},
	{"ingeniero de produccion", "industrial_engineering"},
	{"ingeniero de calidad", "industrial_engineering"},
	{"ingeniero de proyectos", "industrial_engineering"},
	{"ingeniero industrial", "industrial_engineering"},
	{"ingeniera industrial", "industrial_engineering"},
	{"ingeniero electrico", "industrial_engineering"},
	{"ingeniero electromecanico", "industrial_engineering"},
	{"ingeniero mecanico", "industrial_engineering"},
	{"ingeniero civil", "industrial_engineering"},
	{"ingeniero quimico", "industrial_engineering"},
	{"ingeniero de obra", "industrial_engineering"},
	{"ingeniero residente", "industrial_engineering"},
	{"ingeniero de servicio", "industrial_engineering"}, // the field service engineer
	{"delineante", "engineering_design"},
	{"proyectista", "engineering_design"},
	{"oficina tecnica", "engineering_design"},
	{"direccion de arte", "creative"},
	{"director de arte", "creative"},
	{"disenador grafico", "design"},
	{"disenadora grafica", "design"},
	{"disenador", "design"},
	{"disenadora", "design"},

	// Projects, before the engineer of one is read as a plant seat.
	{"jefe de proyecto", "project_management"},
	{"jefe de proyectos", "project_management"},
	{"jefa de proyecto", "project_management"},
	{"jefa de proyectos", "project_management"},
	{"gestor de proyectos", "project_management"},
	{"director de proyectos", "project_management"},
	{"coordinador de proyectos", "project_management"},
	{"gerente de proyectos", "project_management"},
	{"gerente de proyecto", "project_management"},

	// Healthcare. The nursing auxiliary is a nurse, the pharmacy's technician works
	// in healthcare, and the medical representative sells.
	{"representante medico", "sales"},
	{"representantes medicos", "sales"},
	{"visitador medico", "sales"},
	{"visitadora medica", "sales"},
	{"auxiliar de enfermeria", "healthcare"},
	{"tecnico en cuidados auxiliares de enfermeria", "healthcare"},
	{"tcae", "healthcare"},
	{"enfermero", "healthcare"},
	{"enfermera", "healthcare"},
	{"enfermeros", "healthcare"},
	{"enfermeria", "healthcare"},
	{"tecnico radiologo", "healthcare"},
	{"radiologia", "healthcare"},
	{"fisioterapeuta", "healthcare"},
	{"higienista dental", "healthcare"},
	{"higienista bucodental", "healthcare"},
	{"odontologo", "healthcare"},
	{"odontologa", "healthcare"},
	{"dentista", "healthcare"},
	{"farmaceutico", "healthcare"},
	{"farmaceutica", "healthcare"},
	{"auxiliar de farmacia", "healthcare"},
	{"tecnico en farmacia", "healthcare"},
	{"tecnico de farmacia", "healthcare"},
	{"gerocultor", "healthcare"},
	{"gerocultora", "healthcare"},
	{"cuidador", "healthcare"},
	{"cuidadora", "healthcare"},
	{"psicologo", "healthcare"},
	{"psicologa", "healthcare"},
	{"terapeuta ocupacional", "healthcare"},
	{"logopeda", "healthcare"},
	{"veterinario", "healthcare"},
	{"veterinaria", "healthcare"},
	{"optico", "healthcare"},
	{"optometrista", "healthcare"},
	{"toma de muestras", "healthcare"}, // the phlebotomist
	{"tomador de muestras", "healthcare"},
	{"matrona", "healthcare"},
	{"celador", "healthcare"}, // the hospital porter
	{"nutricionista", "healthcare"},

	// Education and social services: the social educator is social work, the
	// classroom and the leisure monitor teach.
	{"educador social", "social_services"},
	{"educadora social", "social_services"},
	{"integrador social", "social_services"},
	{"integradora social", "social_services"},
	{"integracion social", "social_services"},
	{"trabajadora social", "social_services"},
	{"ayuda a domicilio", "healthcare"}, // the home care aide
	{"docente", "education"},
	{"docentes", "education"},
	// A bare "maestro" is also the foreman of a building site or a kitchen ("Maestro
	// de obra", "Maestro de cocina") and a restaurant chain's name ("Juan Maestro").
	{"maestro de obra", "skilled_trades"},
	{"maestra", "education"},
	{"educador", "education"},
	{"educadora", "education"},
	{"educacion infantil", "education"},
	{"profesora", "education"},
	{"profesores", "education"},
	{"monitor de comedor", "education"},
	{"monitor de ocio", "education"},
	{"monitor de tiempo libre", "education"},
	{"monitor deportivo", "education"}, // the main table's "fitness instructor"
	{"monitor de natacion", "education"},
	{"entrenador personal", "personal_services"},

	// Public safety.
	{"vigilante", "public_safety"},
	{"vigilantes", "public_safety"},
	{"guardia de seguridad", "public_safety"},
	{"guardias de seguridad", "public_safety"},
	{"agente de seguridad", "public_safety"},
	{"auxiliar de seguridad", "public_safety"},
	{"escolta", "public_safety"},
	{"socorrista", "public_safety"},
	{"bombero", "public_safety"},
	{"bomberos", "public_safety"},
	{"monitorista", "public_safety"}, // the CCTV operator of a security firm

	// Hospitality. The room attendant ("camarera de pisos") is hospitality, as the main
	// table files the English one.
	{"camarero", "hospitality"},
	{"camarera", "hospitality"},
	{"camareros", "hospitality"},
	{"mesero", "hospitality"},
	{"mesera", "hospitality"},
	{"cocinero", "hospitality"},
	{"cocinera", "hospitality"},
	{"cocineros", "hospitality"},
	{"pinche de cocina", "hospitality"},
	{"friegaplatos", "hospitality"},
	{"lavaplatos", "hospitality"},
	{"jefe de partida", "hospitality"},
	{"barman", "hospitality"},
	{"cantinero", "hospitality"},
	{"pizzero", "hospitality"},

	// The trades.
	{"electricista", "skilled_trades"},
	{"electricistas", "skilled_trades"},
	{"fontanero", "skilled_trades"},
	{"albanil", "skilled_trades"},
	{"albaniles", "skilled_trades"},
	{"carpintero", "skilled_trades"},
	{"soldador", "skilled_trades"},
	{"plegador", "manufacturing"}, // the press-brake operator
	{"plegadores", "manufacturing"},
	{"soldadores", "skilled_trades"},
	{"mecanico", "skilled_trades"},
	{"electromecanico", "skilled_trades"},
	{"electromecanicos", "skilled_trades"},
	{"frigorista", "skilled_trades"},
	{"chapista", "skilled_trades"},
	{"cerrajero", "skilled_trades"},
	{"tornero", "skilled_trades"}, // the main table's "machinist"
	{"fresador", "skilled_trades"},
	{"calderero", "skilled_trades"},
	{"pintor", "skilled_trades"},
	{"instalador", "skilled_trades"},
	{"instaladores", "skilled_trades"},
	{"montador", "skilled_trades"},
	{"encofrador", "skilled_trades"},
	{"peon", "skilled_trades"}, // the main table's "laborer"
	{"oficial de 1a", "skilled_trades"},
	{"oficial 1a", "skilled_trades"},
	{"oficial de primera", "skilled_trades"},
	{"oficial de segunda", "skilled_trades"},
	{"oficial de 2a", "skilled_trades"},
	{"residente de obra", "skilled_trades"},

	// Logistics.
	{"mozo", "logistics"},
	{"mozos", "logistics"},
	{"carretillero", "logistics"},
	{"carretilleros", "logistics"},
	{"almacenista", "logistics"},
	{"bodeguero", "logistics"},
	{"preparador de pedidos", "logistics"},
	{"armador de pedidos", "logistics"},
	// Argentina: the forklift driver is a "clarkista" (after Clark), the courier a
	// "cadete", the customs broker a "despachante". The day labourer ("changarín") is
	// absent: he loads lorries as often as he works a building site.
	{"clarkista", "logistics"},
	{"clarkistas", "logistics"},
	{"cadete", "logistics"},
	{"cadetes", "logistics"},
	{"despachante de aduana", "logistics"},
	{"despachante aduanero", "logistics"},
	{"pickers", "logistics"}, // the main table's "picker", plural
	{"conductor", "logistics"},
	{"conductores", "logistics"},
	{"chofer", "logistics"},
	{"choferes", "logistics"},
	{"repartidor", "logistics"},
	{"repartidores", "logistics"},
	{"transportista", "logistics"},
	{"camionero", "logistics"},
	{"mensajero", "logistics"},
	{"gruista", "logistics"},
	{"almacenero", "logistics"},
	{"montacarguista", "logistics"}, // the forklift driver in Mexico
	{"apilador", "logistics"},       // and in Chile
	{"apiladores", "logistics"},
	{"autoelevador", "logistics"},
	{"maquinista clase d", "logistics"}, // the Chilean forklift licence
	{"maquinista con clase d", "logistics"},
	{"maquinista apilador", "logistics"},

	// Retail.
	{"dependiente", "retail"},
	{"dependienta", "retail"},
	{"dependientes", "retail"},
	{"cajero bancario", "finance"}, // the bank teller
	{"cajera bancaria", "finance"},
	{"cajero", "retail"},
	{"cajera", "retail"},
	{"cajeros", "retail"},
	{"reponedor", "retail"},
	{"reponedora", "retail"},
	{"carnicero", "retail"},
	{"pescadero", "retail"},
	{"charcutero", "retail"},
	{"tablajero", "retail"}, // the butcher counter of a Mexican supermarket
	{"abarrotes", "retail"},
	{"frutas y verduras", "retail"},
	{"personal de cajas", "retail"},
	{"expendedor", "retail"}, // the forecourt attendant, as the main table files "frentista"
	{"punto de venta", "retail"},

	// Personal services.
	{"limpiador", "personal_services"},
	{"limpiadora", "personal_services"},
	{"peluquero", "personal_services"},
	{"peluquera", "personal_services"},
	{"peluqueros", "personal_services"},
	{"peluqueria", "personal_services"},
	{"barbero", "personal_services"},
	{"barberos", "personal_services"},
	{"esteticista", "personal_services"},
	{"manicurista", "personal_services"},
	{"estilista", "personal_services"},
	{"masajista", "personal_services"},
	{"conserje", "personal_services"},
	{"jardinero", "personal_services"}, // the main table's "groundskeeper"

	// Finance, legal, people. The meter reader ("lector de contadores") reads meters.
	{"lector de contadores", categoryNone},
	{"contable", "finance"},
	{"contables", "finance"},
	{"contador", "finance"},
	{"contadora", "finance"},
	{"contabilidad", "finance"},
	{"cuentas por pagar", "finance"},
	{"cuentas por cobrar", "finance"},
	{"tesoreria", "finance"},
	{"facturacion", "finance"},
	{"nomina", "finance"}, // payroll, as the main table files it
	{"nominas", "finance"},
	{"cobranza", "finance"},
	{"cobranzas", "finance"},
	{"cobrador de peaje", categoryNone}, // the toll-booth attendant
	{"cobrador", "finance"},
	{"costos", "finance"},
	{"costes", "finance"},
	{"previsional", "finance"}, // the pension-fund advisor
	{"asesor patrimonial", "finance"},
	// The bank's counter and loan desk. Not a bare "bancario": a Portuguese "Advogado
	// / Bancário" is a banking lawyer.
	{"asesor bancario", "finance"},
	{"asesora bancaria", "finance"},
	{"ejecutivo bancario", "finance"},
	{"ejecutiva bancaria", "finance"},
	{"agente bancario", "finance"},
	{"atencion bancaria", "finance"},
	{"oferta bancaria", "finance"},
	{"colocaciones bancaria", "finance"},
	{"colocaciones bancarias", "finance"},
	{"banquero", "finance"},
	{"asesor financiero", "finance"},
	{"asesora financiera", "finance"},
	{"analista financiero", "finance"},
	{"analista de riesgos", "finance"},
	{"riesgo crediticio", "finance"},
	{"asesor de credito", "finance"}, // the loan officer
	{"asesor fiscal", "finance"},
	{"asesora fiscal", "finance"},
	{"auditor de cuentas", "finance"},
	{"abogado", "legal"},
	{"abogada", "legal"},
	{"abogados", "legal"},
	{"asesor juridico", "legal"},
	{"asesor legal", "legal"},
	{"asistente legal", "legal"},
	{"procurador", "legal"},
	{"reclutador", "recruiting"},
	{"reclutadora", "recruiting"},
	{"reclutamiento", "recruiting"},
	{"seleccion de personal", "recruiting"},
	{"tecnico de seleccion", "recruiting"},
	{"consultor de seleccion", "recruiting"},
	{"atraccion de talento", "recruiting"},
	{"recursos humanos", "hr"},
	{"rrhh", "hr"},
	{"capital humano", "hr"},
	{"talento humano", "hr"},
	{"gestion humana", "hr"},
	{"relaciones laborales", "hr"},
	{"tecnico laboral", "hr"},
	{"asesor laboral", "hr"},

	// Customer service and sales.
	{"telemarketing", "marketing"}, // the main table's "telemarketer"
	{"ventas telefonicas", "sales"},
	{"atencion al cliente", "support"},
	{"servicio al cliente", "support"},
	{"atencion a clientes", "support"},
	{"atencion a cliente", "support"},
	{"agente de atc", "support"},
	{"ejecutivo telefonico", "support"},
	{"asesor telefonico", "support"},
	{"operador telefonico", "support"},
	{"atencion telefonica", "support"},
	{"teleoperador", "support"},
	{"teleoperadora", "support"},
	{"teleoperadores", "support"},
	{"agente telefonico", "support"},
	{"postventa", "support"},
	{"agente de seguros", "sales"},
	{"asesor de seguros", "sales"},
	{"consultor de seguros", "sales"},
	{"agente inmobiliario", "sales"},
	{"asesor inmobiliario", "sales"},
	{"asesora inmobiliaria", "sales"},
	{"ejecutivo de cuenta", "sales"},
	{"ejecutivo de cuentas", "sales"},
	{"ejecutiva de cuenta", "sales"},
	{"ejecutiva de cuentas", "sales"},
	{"cuentas clave", "sales"},
	{"ejecutivo pyme", "sales"},
	{"vendedor", "sales"}, // the main table's "sales associate"
	{"vendedora", "sales"},
	{"vendedores", "sales"},
	// Argentina's pre-seller, who takes the shop's order before the delivery, and the
	// in-store promoter who also sells.
	{"preventista", "sales"},
	{"preventistas", "sales"},
	{"promovendedor", "sales"},
	{"promovendedora", "sales"},
	{"promovendedores", "sales"},
	{"mercadotecnia", "marketing"},
	{"publicidad", "marketing"},
	{"redes sociales", "marketing"},
	{"compras", "operations"}, // the main table's "buyer" and "purchasing manager"
	{"comprador", "operations"},
	{"compradora", "operations"},

	// Band 3: the fields that give a generic head its category.
	{"ventas", "sales"},
	{"venta", "sales"},
	{"financiero", "finance"},
	{"financiera", "finance"},
	{"operaciones", "operations"}, // the main table's "operations manager"
	{"equipo medico", categoryNone},
	{"equipos medicos", categoryNone},
	{"dispositivos medicos", categoryNone},
	// "Médico" is also the adjective of the equipment a salesperson sells ("Ejecutivo
	// de ventas equipo médico"), so the physician sits below the sales field.
	{"medico", "healthcare"},
	{"medica", "healthcare"},
	{"medicos", "healthcare"},
	{"mantenimiento", "skilled_trades"}, // the main table's "maintenance technician"
	{"obra civil", "skilled_trades"},
	{"jefe de obra", "skilled_trades"}, // the site manager, filed with the trades
	{"jefa de obra", "skilled_trades"},
	{"encargado de obra", "skilled_trades"},
	{"encargada de obra", "skilled_trades"},
	{"construccion", "skilled_trades"},
	{"produccion audiovisual", "creative"},
	{"produccion", "manufacturing"},
	{"de planta", "manufacturing"},
	{"de fabrica", "manufacturing"},
	{"envasado", "manufacturing"},
	{"envasador", "manufacturing"},
	{"manipulador", "manufacturing"}, // the packing-house hand of a fruit or food plant
	{"manipuladores", "manufacturing"},
	{"montaje", "manufacturing"},
	{"control de calidad", "manufacturing"},
	{"tecnico de calidad", "manufacturing"}, // the main table's "quality technician"
	{"tecnica de calidad", "manufacturing"},
	{"inspector de calidad", "manufacturing"},
	{"logistica", "logistics"},
	{"almacen", "logistics"},
	{"de bodega", "logistics"}, // the warehouse in Chile and Mexico; a bare "bodega" is also a winery
	{"de deposito", "logistics"},
	{"de reparto", "logistics"}, // not bare: the Italian "reparto" is a department ("Capo reparto")
	{"cocina", "hospitality"},
	{"hosteleria", "hospitality"},
	{"restaurante", "hospitality"},
	{"tienda", "retail"},
	{"tiendas", "retail"},
	{"supermercado", "retail"},
	{"limpieza", "personal_services"},
	{"aseo", "personal_services"},

	// Band 4: the generic heads that name a desk on their own. "Centro comercial" is the
	// shopping mall and "ingeniero comercial" a Chilean business degree, so neither is
	// the salesperson; both sit after every role noun above, so "Cajero centro
	// comercial" still resolves.
	{"centro comercial", categoryNone},
	{"ingeniero comercial", categoryNone},
	{"comercial", "sales"},
	{"comerciales", "sales"},
	// Argentina's housekeeper: a hotel's by default, but after the fields, so "Mucama de
	// limpieza en casa particular" is the cleaner the field names.
	{"mucama", "hospitality"},
	{"mucamas", "hospitality"},
	{"administrativo", "administration"},
	{"administrativa", "administration"},
	{"administrativos", "administration"},
	{"auxiliar de administracion", "administration"},
	{"asistente de direccion", "administration"},
	{"secretario", "administration"},
	{"secretaria", "administration"},
	{"telefonista", "administration"},
	{"grabador de datos", "administration"},
	// A security firm's salesperson is a salesperson, so the guard's field sits below
	// "comercial" and "vendedor".
	{"seguridad privada", "public_safety"},
	{"seguridad patrimonial", "public_safety"},
}
