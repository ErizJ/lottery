-- ----------------------------
-- DataBase
-- ----------------------------
CREATE DATABASE lottery;

use lottery;

-- ----------------------------
-- Table structure for inventory
-- ----------------------------
DROP TABLE IF EXISTS `inventory`;
CREATE TABLE `inventory` (
    `id` int(11) NOT NULL AUTO_INCREMENT COMMENT "奖品id, 自增",
    `created_at` DATETIME(3) NULL DEFAULT CURRENT_TIMESTAMP COMMENT "创建时间",
    `updated_at` DATETIME(3) NULL DEFAULT NULL COMMENT "更新时间",
    `deleted_at` DATETIME(3) NULL DEFAULT NULL COMMENT "删除时间",
    `name` varchar(20) NOT NULL COMMENT "奖品名称",
    `description` varchar(100) NOT NULL DEFAULT "" COMMENT "奖品描述",
    `picture` varchar(200) NOT NULL DEFAULT "0" COMMENT "奖品图片",
    `price` int(11) NOT NULL DEFAULT "0" COMMENT "价值",
    `count` int(11) NOT NULL DEFAULT "0" COMMENT "库存量",
    PRIMARY KEY (`id`)
) ENGINE=InnoDB AUTO_INCREMENT=20 DEFAULT CHARSET=utf8 COMMENT="奖品库存表";

insert into `inventory` (id,name,picture,price,count) values (1,'谢谢参与','img/face.png',0,0);
insert into `inventory` (name,picture,price,count) values ('篮球','img/ball.jpeg',100,1000),('水杯','img/cup.jpeg',80,1000),('电脑','img/laptop.jpeg',6000,200),('平板','img/pad.jpg',4000,300),('手机','img/phone.jpeg',5000,400),('锅','img/pot.jpeg',120,1000),('茶叶','img/tea.jpeg',90,1000),('无人机','img/uav.jpeg',400,100),('酒','img/wine.jpeg',160,500);

-- ----------------------------
-- Table structure for order
-- ----------------------------
DROP TABLE IF EXISTS `order`;
CREATE TABLE `order` (
    `id` int(11) NOT NULL AUTO_INCREMENT COMMENT "订单id, 自增",
    `created_at` DATETIME(3) NULL DEFAULT CURRENT_TIMESTAMP COMMENT "创建时间",
    `updated_at` DATETIME(3) NULL DEFAULT NULL COMMENT "更新时间",
    `deleted_at` DATETIME(3) NULL DEFAULT NULL COMMENT "删除时间",
    `gift_id` int(11) NOT NULL COMMENT "商品id",
    `user_id` int(11) NOT NULL COMMENT "用户id",
    `count` int(11) NOT NULL DEFAULT "1" COMMENT "购买数量",
    PRIMARY KEY (`id`)
) ENGINE=InnoDB AUTO_INCREMENT=189549 DEFAULT CHARSET=utf8mb4 COMMENT="订单表";



