package db

/*

CREATE TABLE `lock` (`id` int AUTO_INCREMENT,`serviceID` bigint NOT NULL,`taskType` varchar(255) NOT NULL,`taskID` varchar(255) NOT NULL, PRIMARY KEY (id));
ALTER TABLE `lock`

CREATE TABLE `tasks` (`id` int AUTO_INCREMENT,`name` varchar(255) NOT NULL,`filename` varchar(255) NOT NULL,`createdAt` timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP,`status` varchar(255) NOT NULL DEFAULT 'created',`serviceID` int,`userID` varchar(255), PRIMARY KEY (id));

*/
